package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/hashicorp/go-version"
	jar "github.com/vadikgo/goplugins/lib"
)

// Plugin Plugin info
type Plugin struct {
	Name    string `yaml:"name"`
	Title   string `yaml:"title,omitempty"`
	Version string `yaml:"version,omitempty"`
	Lock    bool   `yaml:"version_lock,omitempty"`
}

// ShortPluginInfo plugin info for dependencies
type ShortPluginInfo struct {
	Name    string
	Version string
}

// PluginInfo Full plugin info
type PluginInfo struct {
	Name               string
	LongName           string
	Version            string
	Dependencies       []ShortPluginInfo
	JenkinsVersion     string
	MinimumJavaVersion string
}

var (
	latestURL         = "https://updates.jenkins-ci.org/latest"
	versionURL        = "https://updates.jenkins-ci.org/download/plugins"
	cache             = make(map[string]PluginInfo)
	jenkinsVersion, _ = version.NewVersion("2.222.2")
	pluginsYaml       = "jenkins_plugins_test.yml"
)

func readPluginInfo(name string, version string) PluginInfo {
	if cached, ok := cache[name+version]; ok {
		return cached
	}
	var url string
	if version != "" {
		url = fmt.Sprintf("%s/%s/%s/%s.hpi", versionURL, name, version, name)
	} else {
		url = fmt.Sprintf("%s/%s.hpi", latestURL, name)
	}
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatalf("error: %v get URL %s", err, url)
	}

	// Download plugin to temporary file
	hpifile, err := ioutil.TempFile("", "jenkins")
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	defer os.Remove(hpifile.Name())

	// Write the body to file
	_, err = io.Copy(hpifile, resp.Body)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	manifest, err := jar.ReadFile(hpifile.Name())
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	dependencies := make([]ShortPluginInfo, 0)
	if pluginDependencies, ok := manifest["Plugin-Dependencies"]; ok {
		for _, dep := range strings.Split(pluginDependencies, ",") {
			// workflow-cps:2.80;resolution:=optional
			pluginInfo := strings.Split(dep, ";")
			if (len(pluginInfo) == 1) || (len(pluginInfo) > 1 && pluginInfo[1] != "resolution:=optional") {
				pluginNameVer := strings.Split(pluginInfo[0], ":")
				dependencies = append(dependencies, ShortPluginInfo{Name: pluginNameVer[0], Version: pluginNameVer[1]})
			}
		}
	}
	newPluginInfo := PluginInfo{
		Name:               manifest["Short-Name"],
		LongName:           manifest["Long-Name"],
		Version:            manifest["Plugin-Version"],
		Dependencies:       dependencies,
		JenkinsVersion:     manifest["Jenkins-Version"],
		MinimumJavaVersion: manifest["Minimum-Java-Version"],
	}
	cache[manifest["Short-Name"]+manifest["Plugin-Version"]] = newPluginInfo
	return newPluginInfo
}

func isAddPlugin(pluginList map[string]PluginInfo, hpiInfo PluginInfo) bool {
	// Check is plugin cant be added to pluginList
	if pl, ok := pluginList[hpiInfo.Name]; ok {
		v1, _ := version.NewVersion(pl.Version)
		v2, _ := version.NewVersion(hpiInfo.Version)
		if v1.GreaterThan(v2) {
			return false
		}
	}
	jv, _ := version.NewVersion(hpiInfo.JenkinsVersion)
	if jenkinsVersion.LessThan(jv) {
		return false
	}
	return true
}

func main() {
	yamlFile, err := ioutil.ReadFile(pluginsYaml)
	if err != nil {
		log.Fatalf("Error reading YAML file: %s\n", err)
	}

	var plugins []Plugin
	path, err := yaml.PathString("$.jenkins_plugins[*]")
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	if err := path.Read(strings.NewReader(string(yamlFile)), &plugins); err != nil {
		log.Fatalf("error: %v", err)
	}
	//fmt.Printf("--- plugins:\n%v\n\n", plugins)

	upgraded := make(map[string]PluginInfo)
	var wg sync.WaitGroup
	for _, plg := range plugins {
		wg.Add(1)
		go func(plg Plugin) {
			defer wg.Done()

			var plugVer string
			if plg.Lock {
				plugVer = plg.Version
			} else {
				plugVer = ""
			}
			plgInfo := readPluginInfo(plg.Name, plugVer)
			if isAddPlugin(upgraded, plgInfo) {
				// Check dependency plugins can be installed
				for _, depPlugin := range plgInfo.Dependencies {
					depPluginInfo := readPluginInfo(depPlugin.Name, depPlugin.Version)
					if !isAddPlugin(upgraded, depPluginInfo) {
						return
					}
				}
				// All depencency can be installed
				for _, depPlugin := range plgInfo.Dependencies {
					upgraded[depPlugin.Name] = readPluginInfo(depPlugin.Name, depPlugin.Version)
				}
				upgraded[plg.Name] = plgInfo
			}
		}(plg)
	}
	wg.Wait()

	//fmt.Printf("--- plugins:\n%v\n\n", upgraded)
	// Show plugins delta
	for key, val := range upgraded {
		found := false
		for _, old := range plugins {
			if old.Name == key {
				found = true
				if old.Version != val.Version {
					fmt.Printf("%s: %s -> %s\n", old.Name, old.Version, val.Version)

				} else {
					if old.Lock {
						fmt.Printf("%s: o %s\n", old.Name, old.Version)
					} else {
						fmt.Printf("%s: %s\n", old.Name, old.Version)
					}
				}
			}

		}
		if !found {
			fmt.Printf("%s: + %s\n", val.Name, val.Version)
		}
	}
}

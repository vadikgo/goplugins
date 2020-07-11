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
	fmt.Printf("--- plugins:\n%v\n\n", plugins)

	upgraded := make(map[string]PluginInfo)
	var wg sync.WaitGroup
	for _, plg := range plugins {
		wg.Add(1)
		go func(plg Plugin) {
			defer wg.Done()
			var hpiInfo PluginInfo
			if plg.Lock {
				hpiInfo = readPluginInfo(plg.Name, plg.Version)
			} else {
				hpiInfo = readPluginInfo(plg.Name, "")
			}
			if pl, ok := upgraded[plg.Name]; ok {
				v1, _ := version.NewVersion(pl.Version)
				v2, _ := version.NewVersion(hpiInfo.Version)
				jv, _ := version.NewVersion(hpiInfo.JenkinsVersion)
				if v1.LessThan(v2) && jenkinsVersion.GreaterThan(jv) {
					upgraded[plg.Name] = hpiInfo
				}
			} else {
				v1, _ := version.NewVersion(plg.Version)
				v2, _ := version.NewVersion(hpiInfo.Version)
				jv, _ := version.NewVersion(hpiInfo.JenkinsVersion)
				if v1.LessThan(v2) && jenkinsVersion.GreaterThan(jv) {
					upgraded[plg.Name] = hpiInfo
				} else {
					upgraded[plg.Name] = readPluginInfo(plg.Name, plg.Version)
				}
			}
		}(plg)
	}
	wg.Wait()

	fmt.Printf("--- plugins:\n%v\n\n", upgraded)
}

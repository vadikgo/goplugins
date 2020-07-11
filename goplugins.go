package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/zenthangplus/goccm"

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

// SafePluginInfo Thread safe plugins cache
type SafePluginInfo struct {
	mux     sync.Mutex
	plugins map[string]PluginInfo
}

var (
	latestURL      = "https://updates.jenkins-ci.org/latest"
	versionURL     = "https://updates.jenkins-ci.org/download/plugins"
	cache          = SafePluginInfo{plugins: make(map[string]PluginInfo)}
	upgraded       = SafePluginInfo{plugins: make(map[string]PluginInfo)}
	jenkinsVersion = "2.222.2"
	pluginsYaml    = "jenkins_plugins_test.yml"
	gomax          = runtime.GOMAXPROCS(0) * 2 // goroutines to run concurrently
	// gomax = 1
)

// SetValue Set Value in cache thread safe
func (c *SafePluginInfo) SetValue(key string, info PluginInfo) {
	c.mux.Lock()
	c.plugins[key] = info
	c.mux.Unlock()
}

// GetValue returns the current value of the counter for the given key.
func (c *SafePluginInfo) GetValue(key string) PluginInfo {
	c.mux.Lock()
	defer c.mux.Unlock()
	return c.plugins[key]
}

// HasKey check key exists
func (c *SafePluginInfo) HasKey(key string) bool {
	c.mux.Lock()
	defer c.mux.Unlock()
	if _, ok := c.plugins[key]; ok {
		return true
	}
	return false
}

func readPluginInfo(name string, version string) PluginInfo {
	if cache.HasKey(name + version) {
		return cache.GetValue(name + version)
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

	if resp.StatusCode == 404 {
		// Custom plugins workaround
		newPluginInfo := PluginInfo{
			Name:               name,
			LongName:           name,
			Version:            version,
			Dependencies:       make([]ShortPluginInfo, 0),
			JenkinsVersion:     jenkinsVersion,
			MinimumJavaVersion: "1.8",
		}
		cache.SetValue(name+version, newPluginInfo)
		return newPluginInfo
	}
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
	if _, ok := manifest["Jenkins-Version"]; !ok {
		manifest["Jenkins-Version"] = jenkinsVersion
	}
	newPluginInfo := PluginInfo{
		Name:               manifest["Short-Name"],
		LongName:           manifest["Long-Name"],
		Version:            manifest["Plugin-Version"],
		Dependencies:       dependencies,
		JenkinsVersion:     manifest["Jenkins-Version"],
		MinimumJavaVersion: manifest["Minimum-Java-Version"],
	}
	cache.SetValue(manifest["Short-Name"]+manifest["Plugin-Version"], newPluginInfo)
	return newPluginInfo
}

func isAddPlugin(hpiInfo PluginInfo) bool {
	// Check is plugin can be added to pluginList
	if upgraded.HasKey(hpiInfo.Name) {
		v1, _ := version.NewVersion(upgraded.GetValue(hpiInfo.Name).Version)
		v2, _ := version.NewVersion(hpiInfo.Version)
		if v1.GreaterThan(v2) {
			return false
		}
	}
	jv, err := version.NewVersion(hpiInfo.JenkinsVersion)
	if err != nil {
		log.Fatalf("error: %v %s", err, hpiInfo.JenkinsVersion)
	}
	currentJenkins, err := version.NewVersion(jenkinsVersion)
	if err != nil {
		log.Fatalf("error: %v %s", err, jenkinsVersion)
	}
	if currentJenkins.LessThan(jv) {
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
	// fmt.Printf("--- plugins:\n%v\n\n", plugins)

	// Limit 16 goroutines to run concurrently.
	c := goccm.New(gomax)
	for _, plg := range plugins {
		c.Wait()
		go func(plg Plugin) {
			defer c.Done()

			var plugVer string
			if plg.Lock {
				plugVer = plg.Version
			} else {
				plugVer = ""
			}
			plgInfo := readPluginInfo(plg.Name, plugVer)
			if plgInfo.Version == "" {
				plgInfo.Version = plg.Version
			}

			if !isAddPlugin(plgInfo) {
				// Copy old plugin if cant update to latest
				plgInfo = readPluginInfo(plg.Name, plg.Version)
				if plgInfo.Version == "" {
					plgInfo.Version = plg.Version
				}
			}
			// Check dependency plugins can be installed
			for _, depPlugin := range plgInfo.Dependencies {
				depPluginInfo := readPluginInfo(depPlugin.Name, depPlugin.Version)
				if !isAddPlugin(depPluginInfo) {
					return
				}
			}
			// All depencency can be installed
			for _, depPlugin := range plgInfo.Dependencies {
				upgraded.SetValue(depPlugin.Name, readPluginInfo(depPlugin.Name, depPlugin.Version))
			}
			upgraded.SetValue(plg.Name, plgInfo)
		}(plg)
	}
	c.WaitAllDone()

	keys := make([]string, 0, len(upgraded.plugins))
	for k := range upgraded.plugins {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// fmt.Printf("--- plugins:\n%v\n\n", upgraded)

	// Show plugins delta as text
	// + - plugin added as dependency
	// o - plugin version locked
	// x.x -> y.y - plugin upgraded
	for _, key := range keys {
		found := false
		for _, old := range plugins {
			if old.Name == key {
				found = true
				if old.Version != upgraded.GetValue(key).Version {
					fmt.Printf("%s: %s -> %s\n", old.Name, old.Version, upgraded.GetValue(key).Version)
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
			fmt.Printf("%s: + %s\n", upgraded.GetValue(key).Name, upgraded.GetValue(key).Version)
		}
	}
}

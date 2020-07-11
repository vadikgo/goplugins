package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
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
	latestURL  = "https://updates.jenkins-ci.org/latest"
	versionURL = "https://updates.jenkins-ci.org/download/plugins"
	cache      = make(map[string]PluginInfo)
)

func readPluginInfo(name string, version string) PluginInfo {
	if cached, ok := cache[name]; ok {
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
	fmt.Println("status", resp.Status)
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

	var dependencies []ShortPluginInfo
	for _, dep := range strings.Split(manifest["Plugin-Dependencies"], ",") {
		// workflow-cps:2.80;resolution:=optional
		pluginInfo := strings.Split(dep, ";")
		if (len(pluginInfo) == 1) || (len(pluginInfo) > 1 && pluginInfo[1] != "resolution:=optional") {
			pluginNameVer := strings.Split(pluginInfo[0], ":")
			dependencies = append(dependencies, ShortPluginInfo{Name: pluginNameVer[0], Version: pluginNameVer[1]})
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
	cache[manifest["Short-Name"]] = newPluginInfo
	return newPluginInfo
}

func main() {
	yamlFile, err := ioutil.ReadFile("jenkins_plugins.yml")
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

	info := readPluginInfo("kubernetes", "")
	fmt.Printf("%v\n", info)
	info = readPluginInfo("kubernetes", "1.26.3")
	fmt.Printf("%v\n", info)
}

/*
Copyright 2015 Kelsey Hightower All rights reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/libcompose/config"
	"github.com/docker/libcompose/project"
	"gopkg.in/yaml.v2"
)

func parseDockerCompose() *project.Project {
	composeFile := composeFilePath + "docker-compose.yml"
	p := project.NewProject(&project.Context{
		ProjectName:  "kube",
		ComposeFiles: []string{composeFile},
	}, nil, &config.ParseOptions{})

	if err := p.Parse(); err != nil {
		log.Fatalf("Failed to parse the compose project from %s: %v", composeFile, err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create the output directory %s: %v", outputDir, err)
	}

	if p.ServiceConfigs == nil {
		log.Fatalf("No service config found, aborting")
	}
	return p
}

func writeFile(shortName string, sufix string, object interface{}) {
	data, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal file %s-%s: %v", shortName, sufix, err)
	}
	if asJSON {
		// Save the replication controller for the Docker compose service to the
		// configs directory.
		outputFileName := fmt.Sprintf("%s-%s.json", shortName, sufix)
		outputFilePath := filepath.Join(outputDir, outputFileName)
		if err := ioutil.WriteFile(outputFilePath, data, 0644); err != nil {
			log.Fatalf("Failed to wrtie file %s: %v", outputFileName, err)
		}
		fmt.Println(outputFilePath)
	} else {
		// Save the replication controller to Yaml file
		var exp interface{}
		// because yaml is not directly usable from api, we can unmarshal json to interface{}
		// and then write yaml file
		// yaml segfaults on serializing srv directly
		json.Unmarshal(data, &exp)
		data, err = yaml.Marshal(exp)
		if err != nil {
			log.Fatalf("Failed to marshal file %s-%s: %v", shortName, sufix, err)
		}
		// Remove mark for the right yaml marshal of variables
		fixedData := strings.Replace(string(data), "\\rYMLMARSHALBUG", "", -1)
		// Comment ExternalName in order to be compatible with Rancher
		fixedData = strings.Replace(fixedData, "ExternalName:", "# ExternalName:", -1)
		// Save the replication controller for the Docker compose service to the
		// configs directory.
		outputFileName := fmt.Sprintf("%s-%s.yml", shortName, sufix)
		outputFilePath := filepath.Join(outputDir, outputFileName)
		if err := ioutil.WriteFile(outputFilePath, []byte(fixedData), 0644); err != nil {
			log.Fatalf("Failed to write service %s: %v", outputFileName, err)
		}
		fmt.Println(outputFilePath)
	}
}

func processDockerCompose(dockerCompose *project.Project, rancherCompose map[interface{}]interface{}) {
	for _, name := range dockerCompose.ServiceConfigs.Keys() {
		service, ok := dockerCompose.ServiceConfigs.Get(name)
		if !ok {
			log.Fatalf("Failed to get key %s from config", name)
		}

		shortName := name
		if len(name) > 24 {
			shortName = name[0:24]
		}

		rc := createReplicationController(name, shortName, service, rancherCompose)
		cleanServices(name, rancherCompose)
		writeFile(shortName, "rc", rc)

		srv := createService(shortName, service, rc)
		writeFile(shortName, "srv", srv)

	}
}

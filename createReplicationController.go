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
	"log"
	"strconv"
	"strings"

	"github.com/docker/libcompose/config"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
)

func createReplicationController(name string, shortName string, service *config.ServiceConfig, rancherCompose map[interface{}]interface{}) *api.ReplicationController {
	rc := &api.ReplicationController{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "ReplicationController",
			APIVersion: "v1",
		},
		ObjectMeta: api.ObjectMeta{
			Name:      shortName,
			Namespace: calculateNamespace(),
			Labels:    map[string]string{"service": shortName},
		},
		Spec: api.ReplicationControllerSpec{
			Replicas: configureScale(name, rancherCompose),
			Selector: map[string]string{"service": shortName},
			Template: &api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Labels: configureLabels(shortName, service),
				},
				Spec: api.PodSpec{
					NodeSelector: configureAffinity(shortName, service), //map[string]string{"cloud.google.com/gke-local-ssd": "true"},
					Containers: []api.Container{
						{
							Name:           shortName,
							Image:          service.Image,
							Args:           service.Command,
							Command:        service.Entrypoint,
							Ports:          configurePorts(name, service),
							Env:            configureVariables(service),
							ReadinessProbe: configureHealthCheck(name, rancherCompose),
						},
					},
					RestartPolicy: configureRestartPolicy(name, service),
				},
			},
		},
	}
	rc.Spec.Template.Spec.Containers[0].VolumeMounts, rc.Spec.Template.Spec.Volumes = configureVolumes(service)
	return rc
}

func configurePorts(name string, service *config.ServiceConfig) []api.ContainerPort {
	var ports []api.ContainerPort
	for _, port := range service.Ports {
		// Check if we have to deal with a mapped port
		port = strings.Trim(port, "\"")
		port = strings.TrimSpace(port)
		if strings.Contains(port, ":") {
			parts := strings.Split(port, ":")
			port = parts[1]
		}
		portNumber, err := strconv.ParseInt(port, 10, 32)
		if err != nil {
			log.Fatalf("Invalid container port %s for service %s", port, name)
		}
		ports = append(ports, api.ContainerPort{ContainerPort: int32(portNumber)})
	}
	return ports
}

func configureVariables(service *config.ServiceConfig) []api.EnvVar {
	// Configure the container ENV variables
	var envs []api.EnvVar
	envs = append(envs, api.EnvVar{Name: "NAMESPACE", Value: calculateNamespace()})
	for _, env := range service.Environment {
		if strings.Contains(env, "=") {
			parts := strings.Split(env, "=")
			ename := parts[0]
			var evalue string
			if asJSON == true {
				evalue = parts[1]
			} else {
				// I do this in orer to force the yml marshaller to put quotes in all variables
				// It will be removed later when writing file
				evalue = "\rYMLMARSHALBUG" + parts[1]
			}
			envs = append(envs, api.EnvVar{Name: ename, Value: evalue})
		}
	}
	return envs
}

func configureLabels(shortName string, service *config.ServiceConfig) map[string]string {
	labels := make(map[string]string)
	labels["service"] = shortName
	for index, label := range service.Labels {
		invalid := strings.Index(index, "io.rancher.scheduler")
		if invalid >= 0 {
			log.Printf("Ignoring label %s: %s", index, label)
		} else {
			labels[index] = label
		}
	}
	return labels
}

func configureAffinity(shortName string, service *config.ServiceConfig) map[string]string {
	affinity := make(map[string]string)
	for index, label := range service.Labels {
		if index == "io.rancher.scheduler.affinity:host_label" {
			splitted := strings.Split(label, "=")
			if len(splitted) != 2 {
				log.Printf("Wrong label value %s: %s", index, label)
			} else {
				affinity[splitted[0]] = splitted[1]
			}
		} else {
			//log.Printf("Ignoring label %s: %s", index, label)
		}
	}
	return affinity
}

func configureVolumes(service *config.ServiceConfig) ([]api.VolumeMount, []api.Volume) {
	var volumemounts []api.VolumeMount
	var volumes []api.Volume
	if service.Volumes != nil {
		for _, volumestr := range service.Volumes.Volumes {
			parts := strings.Split(volumestr.String(), ":")
			if len(parts) < 2 {
				log.Fatalf("Volumes without host path are not supported: %s", parts)
			}
			partHostDir := parts[0]
			partContainerDir := parts[1]
			partReadOnly := false
			if len(parts) > 2 {
				for _, partOpt := range parts[2:] {
					switch partOpt {
					case "ro":
						partReadOnly = true
						break
					case "rw":
						partReadOnly = false
						break
					}
				}
			}
			partName := strings.Replace(partHostDir, "/", "", -1)
			if len(parts) > 2 {
				volumemounts = append(volumemounts, api.VolumeMount{Name: partName, ReadOnly: partReadOnly, MountPath: partContainerDir})
			} else {
				volumemounts = append(volumemounts, api.VolumeMount{Name: partName, ReadOnly: partReadOnly, MountPath: partContainerDir})
			}
			source := &api.HostPathVolumeSource{
				Path: partHostDir,
			}
			vsource := api.VolumeSource{HostPath: source}
			volumes = append(volumes, api.Volume{Name: partName, VolumeSource: vsource})
		}
	}
	return volumemounts, volumes
}

func configureRestartPolicy(name string, service *config.ServiceConfig) api.RestartPolicy {
	restartPolicy := api.RestartPolicyAlways
	switch service.Restart {
	case "", "always":
		restartPolicy = api.RestartPolicyAlways
	case "no":
		restartPolicy = api.RestartPolicyNever
	case "on-failure":
		restartPolicy = api.RestartPolicyOnFailure
	default:
		log.Fatalf("Unknown restart policy %s for service %s", service.Restart, name)
	}
	return restartPolicy
}

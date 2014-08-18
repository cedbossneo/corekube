package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type KeyValue map[string]interface{}

type KeyValueGroup []KeyValue // a slice of KeyValue's

type EtcdMachine struct {
	name      string
	state     string
	clientURL string
	peerURL   string
}

type EtcdMachineGroup []EtcdMachine // a slice of EtcdMachine's

// Modify EtcdMachine
func (e *EtcdMachine) SetProperties(machineData KeyValue) {
	e.name = machineData["name"].(string)
	e.state = machineData["state"].(string)
	e.clientURL = machineData["clientURL"].(string)
	e.peerURL = machineData["peerURL"].(string)
}

// Retrieve string of EtcdMachine
func (e *EtcdMachine) String() string {
	output := fmt.Sprintf("Name: %s | State: %s | ClientURL: %s | PeerURL: %s",
		e.name,
		e.state,
		e.clientURL,
		e.peerURL,
	)
	return output
}

// Check for errors and panic, if found
func checkForErrors(err error) {
	if err != nil {
		log.Fatal("%s", err)
	}
}

// Get the IP address of the docker host as this is run from within container
func getDockerHostIP() string {
	cmd := fmt.Sprintf("netstat -nr | grep '^0\\.0\\.0\\.0' | awk '{print $2}'")
	out, err := exec.Command("sh", "-c", cmd).Output()
	checkForErrors(err)

	ip := string(out)
	ip = strings.Replace(ip, "\n", "", -1)
	return ip
}

// Compose the etcd API host:port location
func getEtcdAPI(host string, port string) string {
	return fmt.Sprintf("http://%s:%s", host, port)
}

func httpGetRequest(url string) []byte {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal("%s", err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Fatal("%s", err)
	}

	return body
}

func waitForMachines(machineCount int) {
	log.Printf("%d", machineCount)
}

func getState(machines *EtcdMachineGroup) string {
	hostname := os.Getenv("DOCKERHOST_HOSTNAME")
	hostname = strings.Split(hostname, ".")[0]

	for _, machine := range *machines {
		log.Printf("%s -- %s", machine.name, hostname)
		if machine.name == hostname {
			return machine.state
		}
	}

	return ""
}

func setupFlags() int {
	machineCount :=
		flag.Int("machine_count", 0, "Number of machines to watch for")
	flag.Parse()

	return *machineCount
}

// Access the CoreOS / docker etcd API to extract machine information
func main() {
	if machineCount := setupFlags(); machineCount > 0 {
		waitForMachines(machineCount)
	}

	// Local etcd API host & port
	port := "7001"
	etcdAPI := getEtcdAPI(getDockerHostIP(), port)

	// Request path listing etcd machines in cluster
	etcdAPIPath := "v2/admin/machines"
	url := fmt.Sprintf("%s/%s", etcdAPI, etcdAPIPath)

	jsonResponse := httpGetRequest(url)

	// Decode the JSON returned
	var machineDataGroup KeyValueGroup
	err := json.Unmarshal(jsonResponse, &machineDataGroup)
	checkForErrors(err)

	// Use machine data to create local objects of the etcd machines
	machines := EtcdMachineGroup{}
	for _, machineData := range machineDataGroup {
		machine := EtcdMachine{}
		machine.SetProperties(machineData)
		machines = append(machines, machine)
	}

	for _, machine := range machines {
		log.Printf("%s\n", machine.String())
	}

	state := getState(&machines)
	log.Printf("%s", state)
}

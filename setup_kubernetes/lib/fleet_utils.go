package lib

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/coreos/fleet/unit"
)

var FLEET_API_VERSION string = "v1-alpha"
var FLEET_API_PORT string = "10001"

// Types Result, ResultNode, NodeResult & Node adapted from:
// https://github.com/coreos/fleet/blob/master/etcd/result.go

type Map map[string]interface{}

type Result struct {
	Action string
	Node   ResultNode
}

type ResultNodes []ResultNode
type ResultNode struct {
	Key           string
	Dir           bool
	Nodes         ResultNodes
	ModifiedIndex int
	CreatedIndex  int
}

type NodeResult struct {
	Action string
	Node   Node
}

type Node struct {
	Key           string
	Value         string
	Expiration    string
	Ttl           int
	ModifiedIndex int
	CreatedIndex  int
}

type FleetMachines []FleetMachine
type FleetMachine struct {
	ID             string
	PublicIP       string
	Metadata       Map
	Version        string
	TotalResources Map
}

type FleetUnitState struct {
	Hash               string
	MachineID          string
	Name               string
	SystemdActiveState string
	SystemdLoadState   string
	SystemdSubState    string
}

type FleetUnitStates struct {
	States []FleetUnitState
}

func (f FleetMachine) String() string {
	output := fmt.Sprintf(
		"Machine:\n\t\t-- ID: %s\n\t\t-- IP: %s\n\t\t-- Metadata: %s\n",
		f.ID,
		f.PublicIP,
		f.Metadata.String(),
	)
	return output
}

func (m Map) String() string {
	output := ""
	for k, v := range m {
		output += fmt.Sprintf("(%s => %s) ", k, v)
	}
	return output
}

func lowerCasingOfUnitOptionsStr(json_str string) string {
	json_str = strings.Replace(json_str, "Section", "section", -1)
	json_str = strings.Replace(json_str, "Name", "name", -1)
	json_str = strings.Replace(json_str, "Value", "value", -1)

	return json_str
}

func createUnitFiles(fleetMachine *FleetMachine) {
	// Create all systemd unit files from templates
	path := "/units/kubernetes_units"

	// Start all systemd unit files in specified path via fleet
	unitPathInfo := []map[string]string{}
	unitPathInfo = append(unitPathInfo, map[string]string{
		"path":        path + "/download",
		"activeState": "active", "subState": "exited"})
	unitPathInfo = append(unitPathInfo, map[string]string{
		"path":        path + "/roles",
		"activeState": "active", "subState": "running"})

	/*
		perm := os.FileMode(os.ModeDir)

			for _, v := range unitPathInfo {
				err := os.RemoveAll(v["path"])
				checkForErrors(err)

				os.MkdirAll(v["path"], perm)
			}
	*/

	switch fleetMachine.Metadata["kubernetes_role"] {
	case "master":
		createMasterUnits(fleetMachine, unitPathInfo)
	case "minion":
		createMinionUnits(fleetMachine, unitPathInfo)
	}
	log.Printf("Created unit files for: %s\n", fleetMachine.ID)
}

func StartUnitsInDir(path string) {
	files, _ := ioutil.ReadDir(path)

	for _, f := range files {
		statusCode := 0
		for statusCode != 204 {
			unitpath := fmt.Sprintf("%s/units/%s", FLEET_API_VERSION, f.Name())
			url := getFullAPIURL(FLEET_API_PORT, unitpath)
			filepath := fmt.Sprintf("%s/%s", path, f.Name())
			readfile, err := ioutil.ReadFile(filepath)
			checkForErrors(err)

			content := string(readfile)
			u, _ := unit.NewUnitFile(content)

			options_bytes, _ := json.Marshal(u.Options)
			options_str := lowerCasingOfUnitOptionsStr(string(options_bytes))

			json_str := fmt.Sprintf(
				`{"name": "%s", "desiredState":"launched", "options": %s}`,
				f.Name(),
				options_str)

			resp := httpPutRequest(url, []byte(json_str), true)
			statusCode = resp.StatusCode

			if statusCode != 204 {
				time.Sleep(1 * time.Second)
				//log.Printf("curl -H \"Content-Type: application/json\" -X PUT -d %q localhost:10001/v1-alpha/units/%s", json_str, f.Name())
				//body, err := ioutil.ReadAll(resp.Body)
				//log.Printf("Status Code: %s", statusCode)
				//log.Printf("[Error] in HTTP Body: %s - %v", body, err)
				//checkForErrors(err)
			}
		}
	}
}

func stringInSlice(a string, list []os.FileInfo) bool {
	for _, b := range list {
		if b.Name() == a {
			return true
		}
	}
	return false
}

func CheckUnitsState(path, activeState, subState string) {

	var fleetUnitStates FleetUnitStates

	urlPath := fmt.Sprintf("%s/state", FLEET_API_VERSION)
	url := getFullAPIURL(FLEET_API_PORT, urlPath)
	jsonResponse := httpGetRequest(url)
	err := json.Unmarshal(jsonResponse, &fleetUnitStates)
	checkForErrors(err)

	files, _ := ioutil.ReadDir(path)

	totalKubernetesMachines := len(files)
	activeExitedCount := 0
	for activeExitedCount < totalKubernetesMachines {
		for _, unit := range fleetUnitStates.States {
			if stringInSlice(unit.Name, files) &&
				unit.SystemdActiveState == activeState &&
				unit.SystemdSubState == subState {
				activeExitedCount += 1
			}
		}
		if activeExitedCount == totalKubernetesMachines {
			break
		}
		log.Printf("Waiting for (%d) services to be complete "+
			"in fleet. Currently at: (%d)",
			totalKubernetesMachines, activeExitedCount)
		activeExitedCount = 0
		time.Sleep(1 * time.Second)
		jsonResponse := httpGetRequest(url)
		err := json.Unmarshal(jsonResponse, &fleetUnitStates)
		checkForErrors(err)
	}

	log.Printf("Unit files in '%s' have completed", path)
}

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
		"\nMachine:\n-- ID: %s\n-- IP: %s\n-- Metadata: %s\n\n",
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

func StartUnitsInDir(path string) {
	files, _ := ioutil.ReadDir(path)

	for _, f := range files {
		statusCode := 0
		for statusCode != 204 {
			unitpath := fmt.Sprintf("v1-alpha/units/%s", f.Name())
			url := getFullAPIURL("10001", unitpath)
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

	url := getFullAPIURL("10001", "v1-alpha/state")
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

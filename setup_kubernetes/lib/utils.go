package lib

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var ETCD_API_VERSION string = "v2"
var ETCD_CLIENT_PORT string = "4001"

// Check for errors and panic, if found
func checkForErrors(err error) {
	if err != nil {
		pc, fn, line, _ := runtime.Caller(1)
		msg := fmt.Sprintf("[Error] in %s[%s:%d] %v",
			runtime.FuncForPC(pc).Name(), fn, line, err)
		log.Fatal(msg)
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
	checkForErrors(err)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	checkForErrors(err)

	return body
}

func httpPutRequest(
	urlStr string, data interface{}, isJSON bool) *http.Response {
	var req *http.Request

	switch isJSON {
	case true:
		var dataBytes = data.([]byte)
		req, _ := http.NewRequest("PUT", urlStr, bytes.NewBuffer(dataBytes))
		req.Header.Set("Content-Type", "application/json")
	case false:
		//var dataStr = data.(string)
		dataStr := data.(string)
		req, _ = http.NewRequest("PUT", urlStr, bytes.NewBufferString(dataStr))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Content-Length", strconv.Itoa(len(dataStr)))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	checkForErrors(err)

	defer resp.Body.Close()

	return resp
}

func getFullAPIURL(port, etcdAPIPath string) string {
	etcdAPI := getEtcdAPI(getDockerHostIP(), port)
	url := fmt.Sprintf("%s/%s", etcdAPI, etcdAPIPath)
	return url
}

func getFleetMachines(fleetResult *Result) {
	// Issue request to get machines & parse it. Sleep if cluster not ready yet
	path := fmt.Sprintf("%s/keys/_coreos.com/fleet/machines", ETCD_API_VERSION)
	url := getFullAPIURL(ETCD_CLIENT_PORT, path)
	jsonResponse := httpGetRequest(url)
	err := json.Unmarshal(jsonResponse, fleetResult)
	checkForErrors(err)
}

func getMachinesDeployed() []string {
	var machinesDeployedResult NodeResult

	path := fmt.Sprintf("%s/keys/deployed", ETCD_API_VERSION)
	urlStr := getFullAPIURL(ETCD_CLIENT_PORT, path)

	jsonResponse := httpGetRequest(urlStr)
	err := json.Unmarshal(jsonResponse, &machinesDeployedResult)
	checkForErrors(err)

	var machinesDeployed []string
	var machinesDeployedBytes []byte = []byte(machinesDeployedResult.Node.Value)
	err = json.Unmarshal(machinesDeployedBytes, &machinesDeployed)
	checkForErrors(err)

	return machinesDeployed
}

func setMachinesDeployed(id string) {
	path := fmt.Sprintf("%s/keys/deployed", ETCD_API_VERSION)
	urlStr := getFullAPIURL(ETCD_CLIENT_PORT, path)
	data := ""

	switch id {
	case "":
		emptySlice := []string{}
		dataJSON, _ := json.Marshal(emptySlice)
		data = fmt.Sprintf("value=%s", dataJSON)
	default:
		machineIDs := getMachinesDeployed()
		deployed := false

		for _, machineID := range machineIDs {
			if machineID == id {
				deployed = true
			}
		}

		if !deployed {
			machineIDs = append(machineIDs, id)
			data = fmt.Sprintf("value=%s", machineIDs)
		}
	}

	resp := httpPutRequest(urlStr, data, false)
	statusCode := resp.StatusCode

	if statusCode != 200 {
		time.Sleep(1 * time.Second)
		setMachinesDeployed(id)
	}

}

func Run(fleetResult *Result) {
	var fleetMachines FleetMachines

	getFleetMachines(fleetResult)
	totalMachines := len(fleetResult.Node.Nodes)
	setMachinesDeployed("")

	// Get Fleet machines
	//for {
	log.Printf("Current number of machines found: (%d)\n", totalMachines)
	// Get Fleet machines metadata
	for _, resultNode := range fleetResult.Node.Nodes {
		var fleetMachine FleetMachine
		WaitForMetadata(&resultNode, &fleetMachine)
		log.Printf("------------------------------------------------")
		log.Printf(fleetMachine.String())
		setMachinesDeployed(fleetMachine.ID)
		createUnitFiles(&fleetMachine)
		fleetMachines = append(fleetMachines, fleetMachine)
	}

	time.Sleep(500 * time.Millisecond)
	getFleetMachines(fleetResult)
	totalMachines = len(fleetResult.Node.Nodes)
	//}
}

func WaitForMetadata(
	resultNode *ResultNode,
	fleetMachine *FleetMachine,
) {

	// Issue request to get machines & parse it. Sleep if cluster not ready yet
	id := strings.Split(resultNode.Key, "fleet/machines/")[1]
	path := fmt.Sprintf(
		"%s/keys/_coreos.com/fleet/machines/%s/object", ETCD_API_VERSION, id)

	url := getFullAPIURL(ETCD_CLIENT_PORT, path)
	jsonResponse := httpGetRequest(url)

	var nodeResult NodeResult
	err := json.Unmarshal(jsonResponse, &nodeResult)
	checkForErrors(err)

	err = json.Unmarshal(
		[]byte(nodeResult.Node.Value), &fleetMachine)
	checkForErrors(err)

	for len(fleetMachine.Metadata) == 0 ||
		fleetMachine.Metadata["kubernetes_role"] == nil {
		log.Printf("Waiting for machine (%s) metadata to be available "+
			"in fleet...", fleetMachine.ID)
		time.Sleep(500 * time.Millisecond)

		err = json.Unmarshal(
			[]byte(nodeResult.Node.Value), &fleetMachine)
		checkForErrors(err)

	}
}

func FindInfoForRole(
	role string,
	fleetMachines *[]FleetMachine) []string {
	var machines []string

	for _, fleetMachine := range *fleetMachines {
		if fleetMachine.Metadata["kubernetes_role"] == role {
			machines = append(machines, fleetMachine.PublicIP)
		}
	}

	return machines
}

func Usage() {
	fmt.Printf("Usage: %s\n", os.Args[0])
	flag.PrintDefaults()
}

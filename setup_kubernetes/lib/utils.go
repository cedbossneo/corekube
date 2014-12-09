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

func markMachineDeployed(id string) {
	path := fmt.Sprintf("%s/keys/deployed", ETCD_API_VERSION)
	urlStr := getFullAPIURL(ETCD_CLIENT_PORT, path)
	data := fmt.Sprintf("value='%s'", id)

	resp := httpPutRequest(urlStr, data, false)
	statusCode := resp.StatusCode
	if statusCode != 200 {
		time.Sleep(1 * time.Second)
		markMachineDeployed(id)
	}

}

func Run(fleetResult *Result) {
	var fleetMachines FleetMachines

	getFleetMachines(fleetResult)
	totalMachines := len(fleetResult.Node.Nodes)

	// Get Fleet machines
	//for {
	log.Printf("Current number of machines found: (%d)\n", totalMachines)
	// Get Fleet machines metadata
	for _, resultNode := range fleetResult.Node.Nodes {
		var fleetMachine FleetMachine
		WaitForMetadata(&resultNode, &fleetMachine)
		markMachineDeployed(fleetMachine.ID)

		fleetMachines = append(fleetMachines, fleetMachine)
		log.Printf(fleetMachine.String())
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
	fmt.Printf("here0: %s -- %s", ETCD_CLIENT_PORT, ETCD_API_VERSION)
	path := fmt.Sprintf(
		"%s/keys/_coreos.com/fleet/machines/%s/object", ETCD_API_VERSION, id)

	url := getFullAPIURL(ETCD_CLIENT_PORT, path)
	fmt.Printf("here1: %s -- %s", ETCD_CLIENT_PORT, ETCD_API_VERSION)
	jsonResponse := httpGetRequest(url)

	fmt.Printf("here2: %s -- %s", ETCD_CLIENT_PORT, ETCD_API_VERSION)
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

func createMasterUnits(
	fleetMachine *FleetMachine,
	unitPathInfo []map[string]string,
) {

	files := map[string]string{
		"api":        "master-apiserver@.service",
		"controller": "master-controller-manager@.service",
		"scheduler":  "master-scheduler@.service",
		"download":   "master-download-kubernetes@.service",
	}

	// Form apiserver service file from template
	readfile, err := ioutil.ReadFile(
		fmt.Sprintf("/templates/%s", files["api"]))
	checkForErrors(err)
	apiserver := string(readfile)
	apiserver = strings.Replace(apiserver, "<ID>", fleetMachine.ID, -1)

	// Write apiserver service file
	filename := strings.Replace(files["api"], "@", "@"+fleetMachine.ID, -1)
	apiserver_file := fmt.Sprintf("%s/%s", unitPathInfo[1]["path"], filename)
	err = ioutil.WriteFile(apiserver_file, []byte(apiserver), 0644)
	checkForErrors(err)

	// Form controller service file from template
	readfile, err = ioutil.ReadFile(
		fmt.Sprintf("/templates/%s", files["controller"]))
	checkForErrors(err)
	controller := string(readfile)
	controller = strings.Replace(controller, "<ID>", fleetMachine.ID, -1)

	// Write controller service file
	filename = strings.Replace(files["controller"], "@", "@"+fleetMachine.ID, -1)
	controller_file := fmt.Sprintf("%s/%s", unitPathInfo[1]["path"], filename)
	err = ioutil.WriteFile(controller_file, []byte(controller), 0644)
	checkForErrors(err)

	// Form scheduler service file from template
	readfile, err = ioutil.ReadFile(
		fmt.Sprintf("/templates/%s", files["scheduler"]))
	checkForErrors(err)
	scheduler := string(readfile)
	scheduler = strings.Replace(scheduler, "<ID>", fleetMachine.ID, -1)

	// Write scheduler service file
	filename = strings.Replace(files["scheduler"], "@", "@"+fleetMachine.ID, -1)
	scheduler_file := fmt.Sprintf("%s/%s", unitPathInfo[1]["path"], filename)
	err = ioutil.WriteFile(scheduler_file, []byte(scheduler), 0644)
	checkForErrors(err)

	// Form download service file from template
	readfile, err = ioutil.ReadFile(
		fmt.Sprintf("/templates/%s", files["download"]))
	checkForErrors(err)
	download := string(readfile)
	download = strings.Replace(download, "<ID>", fleetMachine.ID, -1)

	// Write download service file
	filename = strings.Replace(files["download"], "@", "@"+fleetMachine.ID, -1)
	download_file := fmt.Sprintf("%s/%s",
		unitPathInfo[0]["path"], filename)
	err = ioutil.WriteFile(download_file, []byte(download), 0644)
	checkForErrors(err)
}

func createMinionUnits(fleetMachine *FleetMachine,
	unitPathInfo []map[string]string,
) {
	files := map[string]string{
		"kubelet":  "minion-kubelet@.service",
		"proxy":    "minion-proxy@.service",
		"download": "minion-download-kubernetes@.service",
	}

	// Form kubelet service file from template
	readfile, err := ioutil.ReadFile(
		fmt.Sprintf("/templates/%s", files["kubelet"]))
	checkForErrors(err)
	kubelet := string(readfile)
	kubelet = strings.Replace(kubelet, "<ID>", fleetMachine.ID, -1)
	kubelet = strings.Replace(kubelet, "<IP_ADDR>", fleetMachine.PublicIP, -1)

	// Write kubelet service file
	filename := strings.Replace(files["kubelet"], "@", "@"+fleetMachine.ID, -1)
	kubelet_file := fmt.Sprintf("%s/%s", unitPathInfo[1]["path"], filename)
	err = ioutil.WriteFile(kubelet_file, []byte(kubelet), 0644)
	checkForErrors(err)

	// Form proxy service file from template
	readfile, err = ioutil.ReadFile(
		fmt.Sprintf("/templates/%s", files["proxy"]))
	checkForErrors(err)
	proxy := string(readfile)
	proxy = strings.Replace(proxy, "<ID>", fleetMachine.ID, -1)

	// Write proxy service file
	filename = strings.Replace(files["proxy"], "@", "@"+fleetMachine.ID, -1)
	proxy_file := fmt.Sprintf("%s/%s", unitPathInfo[1]["path"], filename)
	err = ioutil.WriteFile(proxy_file, []byte(proxy), 0644)
	checkForErrors(err)

	// Form download service file from template
	readfile, err = ioutil.ReadFile(
		fmt.Sprintf("/templates/%s", files["download"]))
	checkForErrors(err)
	download := string(readfile)
	download = strings.Replace(download, "<ID>", fleetMachine.ID, -1)

	// Write download service file
	filename = strings.Replace(files["download"], "@", "@"+fleetMachine.ID, -1)
	download_file := fmt.Sprintf("%s/%s",
		unitPathInfo[0]["path"], filename)
	err = ioutil.WriteFile(download_file, []byte(download), 0644)
	checkForErrors(err)
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

func CreateUnitFiles(
	fleetMachines *[]FleetMachine,
	unitPathInfo []map[string]string,
) {

	perm := os.FileMode(os.ModeDir)

	for _, v := range unitPathInfo {
		err := os.RemoveAll(v["path"])
		checkForErrors(err)

		os.MkdirAll(v["path"], perm)
	}

	for _, fleetMachine := range *fleetMachines {
		switch fleetMachine.Metadata["kubernetes_role"] {
		case "master":
			createMasterUnits(&fleetMachine, unitPathInfo)
		case "minion":
			createMinionUnits(&fleetMachine, unitPathInfo)
		}
	}
	log.Printf("Created systemd unit files for kubernetes deployment")
}

func Usage() {
	fmt.Printf("Usage: %s\n", os.Args[0])
	flag.PrintDefaults()
}

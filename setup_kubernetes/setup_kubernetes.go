package main

import "setup_kubernetes/lib"

// Access the CoreOS / docker etcd API to extract machine information
func main() {
	// Get fleet machines & metadata
	var fleetResult lib.Result
	lib.Wait(&fleetResult)

	/*
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

		lib.CreateUnitFiles(&fleetMachines, unitPathInfo)

		// Start & check state for download & role units
		for _, v := range unitPathInfo {
			lib.StartUnitsInDir(v["path"])
			lib.CheckUnitsState(v["path"], v["activeState"], v["subState"])
		}

		// Register minions with master
		masterIP := lib.FindInfoForRole("master", &fleetMachines)[0]
		minionIPs := lib.FindInfoForRole("minion", &fleetMachines)
		k8sAPI := fmt.Sprintf("http://%s:8080", masterIP)
		for _, minionIP := range minionIPs {
			lib.Register(k8sAPI, minionIP)
		}
	*/
}

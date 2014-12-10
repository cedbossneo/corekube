package main

import "setup_kubernetes/lib"

// Access the CoreOS / docker etcd API to extract machine information
func main() {
	// Get fleet machines & metadata
	var fleetResult lib.Result
	lib.Run(&fleetResult)

	/*
		// Start & check state for download & role units
		for _, v := range unitPathInfo {
			lib.startUnitsInDir(v["path"])
			lib.checkUnitsState(v["path"], v["activeState"], v["subState"])
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

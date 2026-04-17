package app

import "fmt"

type licenseResult struct {
	tier   string
	valid  bool
	reason string
	state  interface{}
}

func loadLicense() *licenseResult {
	return &licenseResult{
		tier:   "open",
		valid:  true,
		reason: "all features enabled",
		state:  nil,
	}
}

func activateLicense(_ string) error {
	fmt.Println("No activation required. Homing by MODUS is free for everyone.")
	fmt.Println("All runtime features are enabled without a license key.")
	return nil
}

func refreshLicense() error {
	fmt.Println("No license refresh required. Homing by MODUS is free for everyone.")
	return nil
}

func deactivateLicense() error {
	fmt.Println("No license state is active. Homing by MODUS is free for everyone.")
	return nil
}

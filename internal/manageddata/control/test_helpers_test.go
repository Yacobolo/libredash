package control_test

import (
	"encoding/json"

	"github.com/Yacobolo/libredash/internal/manageddata"
)

func mustDecodeManifest(value string) manageddata.Manifest {
	var manifest manageddata.Manifest
	if err := json.Unmarshal([]byte(value), &manifest); err != nil {
		panic(err)
	}
	return manifest
}

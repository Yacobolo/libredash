package app

import (
	"context"

	workloadmodule "github.com/Yacobolo/leapview/internal/workload/module"
)

func workloadController(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy) workloadControl {
	if runtime.workloads == nil {
		runtime.workloads, _ = workloadmodule.Build(context.Background(), workloadmodule.Config{Policy: workloadmodule.DefaultConfig()})
	}
	return runtime.workloads
}

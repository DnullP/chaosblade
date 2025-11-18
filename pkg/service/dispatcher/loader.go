package dispatcher

import (
	"fmt"
	"path"

	"github.com/chaosblade-io/chaosblade-spec-go/channel"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	specutil "github.com/chaosblade-io/chaosblade-spec-go/util"

	"github.com/chaosblade-io/chaosblade/exec/cri"
	"github.com/chaosblade-io/chaosblade/exec/jvm"
	"github.com/chaosblade-io/chaosblade/exec/os"
	"github.com/chaosblade-io/chaosblade/version"
)

// LoadDefaultExecutors registers os, cri and jvm executors based on spec definitions.
func (d *Dispatcher) LoadDefaultExecutors() error {
	loaders := []func() error{
		func() error {
			return d.registerModels("", fmt.Sprintf("chaosblade-os-spec-%s.yaml", version.Ver), os.NewExecutor())
		},
		func() error {
			return d.registerModels("", fmt.Sprintf("chaosblade-jvm-spec-%s.yaml", version.Ver), jvm.NewExecutor())
		},
		func() error {
			criSpec := cri.NewCommandModelSpec()
			if err := d.registerModels("", fmt.Sprintf("chaosblade-cri-spec-%s.yaml", version.Ver), cri.NewExecutor()); err != nil {
				return err
			}
			// Attach cri-prefixed JVM actions to align with the CLI behaviour.
			return d.registerModels(criSpec.Name(), fmt.Sprintf("chaosblade-jvm-spec-%s.yaml", version.Ver), cri.NewExecutor())
		},
	}

	for _, loader := range loaders {
		if err := loader(); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) registerModels(parentTarget string, file string, executor spec.Executor) error {
	specFile := path.Join(specutil.GetYamlHome(), file)
	models, err := specutil.ParseSpecsToModel(specFile, executor)
	if err != nil {
		return err
	}
	for i := range models.Models {
		model := &models.Models[i]
		registerModel(d, parentTarget, model)
	}
	return nil
}

func registerModel(d *Dispatcher, parentTarget string, commandSpec spec.ExpModelCommandSpec) {
	cmdName := commandSpec.Name()
	if commandSpec.Scope() != "" && commandSpec.Scope() != "host" && commandSpec.Scope() != "docker" && commandSpec.Scope() != "cri" {
		cmdName = fmt.Sprintf("%s-%s", commandSpec.Scope(), commandSpec.Name())
	}
	for _, action := range commandSpec.Actions() {
		executor := action.Executor()
		if executor != nil {
			executor.SetChannel(channel.NewLocalChannel())
		}
		d.Register(parentTarget, cmdName, action.Name(), executor)
	}
}

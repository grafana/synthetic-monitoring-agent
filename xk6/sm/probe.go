package sm

import (
	"time"

	"github.com/sirupsen/logrus"
	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/lib"
	"go.k6.io/k6/metrics"
)

func init() {
	modules.Register("k6/x/sm", NewRootModule())
}

type RootModule struct{}

var _ modules.Module = &RootModule{}

func NewRootModule() *RootModule {
	return &RootModule{}
}

func (*RootModule) NewModuleInstance(vu modules.VU) modules.Instance {
	return &ModuleInstance{
		vu:     vu,
		prober: &Prober{vu: vu},
	}
}

type ModuleInstance struct {
	vu     modules.VU
	prober *Prober
}

var _ modules.Instance = &ModuleInstance{}

func (mi *ModuleInstance) Exports() modules.Exports {
	return modules.Exports{
		Default: mi.prober,
	}
}

type Prober struct {
	vu modules.VU
}

type Opts struct {
	Method string
}

func (p *Prober) Http(target string, opts Opts) bool {
	ctx := p.vu.Context()
	state := p.vu.State()
	es := lib.GetExecutionState(ctx)
	mFooBar, err := es.Test.Registry.NewMetric("foo_bar", metrics.Counter, metrics.Default)
	if err != nil {
		return false
	}

	state.Logger.WithFields(logrus.Fields{"target": target, "opts": opts}).Info("Prober.Http called")

	// do something interesting here

	metrics.PushIfNotDone(ctx, state.Samples, metrics.Sample{
		Time:  time.Now().UTC(),
		Value: 1,
		TimeSeries: metrics.TimeSeries{
			Metric: mFooBar,
		},
	})

	return true
}

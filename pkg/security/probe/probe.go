package probe

import (
	"bytes"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/iovisor/gobpf/elf"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/ebpf/gobpf"
	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type EventHandler interface {
	HandleEvent(event *Event)
}

type Stats struct {
	Events struct {
		Lost     uint64
		Received uint64
	}
}

type filterCb func(probe *Probe, field string, filters ...eval.Filter) error

type Probe struct {
	*eprobe.Probe
	handler       EventHandler
	resolvers     *Resolvers
	stats         Stats
	eventFilterCb map[string][]filterCb
	enableFilters bool
}

// Capabilities associates eval capabilities with kernel policy flags
type Capabilities struct {
	EvalCapabilities []eval.FilteringCapability
	PolicyFlags      PolicyFlag
}

type KProbe struct {
	*eprobe.KProbe
	EventTypes  map[string]Capabilities
	OnNewFilter filterCb
	PolicyTable string
}

// cache of the syscall prefix depending on kernel version
var syscallPrefix string

func getSyscallFnName(name string) string {
	if syscallPrefix == "" {
		syscall, err := elf.GetSyscallFnName("open")
		if err != nil {
			panic(err)
		}
		syscallPrefix = strings.TrimSuffix(syscall, "open")
	}

	return syscallPrefix + name
}

var AllKProbes = []*KProbe{
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_mkdir",
			EntryFunc: "kprobe/" + getSyscallFnName("mkdir"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("mkdir"),
		},
		EventTypes: map[string]Capabilities{
			"mkdir": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_mkdirat",
			EntryFunc: "kprobe/" + getSyscallFnName("mkdirat"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("mkdirat"),
		},
		EventTypes: map[string]Capabilities{
			"mkdir": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "vfs_mkdir",
			EntryFunc: "kprobe/vfs_mkdir",
		},
		EventTypes: map[string]Capabilities{
			"mkdir": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_rmdir",
			EntryFunc: "kprobe/" + getSyscallFnName("rmdir"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("rmdir"),
		},
		EventTypes: map[string]Capabilities{
			"rmdir": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "vfs_rmdir",
			EntryFunc: "kprobe/vfs_rmdir",
		},
		EventTypes: map[string]Capabilities{
			"rmdir": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_unlink",
			EntryFunc: "kprobe/" + getSyscallFnName("unlink"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("unlink"),
		},
		EventTypes: map[string]Capabilities{
			"unlink": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_unlinkat",
			EntryFunc: "kprobe/" + getSyscallFnName("unlinkat"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("unlinkat"),
		},
		EventTypes: map[string]Capabilities{
			"unlink": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "vfs_unlink",
			EntryFunc: "kprobe/vfs_unlink",
		},
		EventTypes: map[string]Capabilities{
			"unlink": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_rename",
			EntryFunc: "kprobe/" + getSyscallFnName("rename"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("rename"),
		},
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_renameat",
			EntryFunc: "kprobe/" + getSyscallFnName("renameat"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("renameat"),
		},
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_renameat2",
			EntryFunc: "kprobe/" + getSyscallFnName("renameat2"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("renameat2"),
		},
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "vfs_rename",
			EntryFunc: "kprobe/vfs_rename",
		},
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{},
			},
		},
	},
}

func (p *Probe) NewRuleSet(opts eval.Opts) *eval.RuleSet {
	eventCtor := func() eval.Event {
		return NewEvent(p.resolvers)
	}

	return eval.NewRuleSet(&Model{}, eventCtor, opts)
}

func (p *Probe) getTables() []*types.Table {
	tables := []*types.Table{
		{
			Name: "pathnames",
		},
	}

	return append(tables, OpenTables...)
}

func (p *Probe) getPerfMaps() []*types.PerfMap {
	return []*types.PerfMap{
		{
			Name:        "events",
			Handler:     p.handleEvent,
			LostHandler: p.handleLostEvents,
		},
	}
}

func NewProbe(config *config.Config) (*Probe, error) {
	asset := "probe"
	openSyscall := getSyscallFnName("open")
	if !strings.HasPrefix(openSyscall, "SyS_") && !strings.HasPrefix(openSyscall, "sys_") {
		asset += "-syscall-wrapper"
	}

	bytecode, err := Asset(asset + ".o") // ioutil.ReadFile("pkg/security/ebpf/probe.o")
	if err != nil {
		return nil, err
	}

	module, err := gobpf.NewModuleFromReader(bytes.NewReader(bytecode))
	if err != nil {
		return nil, err
	}
	log.Infof("Loaded security agent eBPF module: %+v", module)

	p := &Probe{
		eventFilterCb: make(map[string][]filterCb),
		enableFilters: config.EnableKernelFilters,
	}

	ebpfProbe := &eprobe.Probe{
		Module:   module,
		Tables:   p.getTables(),
		PerfMaps: p.getPerfMaps(),
	}

	for _, kprobe := range AllKProbes {
		ebpfProbe.Kprobes = append(ebpfProbe.Kprobes, kprobe.KProbe)

		for eventType := range kprobe.EventTypes {
			if kprobe.OnNewFilter != nil {
				cbs := p.eventFilterCb[eventType]
				cbs = append(cbs, kprobe.OnNewFilter)
				p.eventFilterCb[eventType] = cbs
			}
		}
	}

	if err := ebpfProbe.Load(); err != nil {
		return nil, err
	}
	p.Probe = ebpfProbe

	dentryResolver, err := NewDentryResolver(ebpfProbe)
	if err != nil {
		return nil, err
	}

	p.resolvers = &Resolvers{
		DentryResolver: dentryResolver,
	}

	return p, nil
}

func (p *Probe) SetEventHandler(handler EventHandler) {
	p.handler = handler
}

func (p *Probe) DispatchEvent(event *Event) {
	if p.handler != nil {
		p.handler.HandleEvent(event)
	}
}

func (p *Probe) GetStats() Stats {
	return p.stats
}

func (p *Probe) ResetStats() {
	p.stats = Stats{}
}

func (p *Probe) handleLostEvents(count uint64) {
	log.Warnf("Lost %d events\n", count)
	atomic.AddUint64(&p.stats.Events.Lost, count)
}

func (p *Probe) handleEvent(data []byte) {
	log.Debugf("Handling dentry event (len %d)", len(data))
	atomic.AddUint64(&p.stats.Events.Received, 1)

	offset := 0
	event := NewEvent(p.resolvers)

	read, err := event.Event.UnmarshalBinary(data)
	if err != nil {
		log.Errorf("failed to decode event")
		return
	}
	offset += read

	read, err = event.Process.UnmarshalBinary(data[offset:])
	if err != nil {
		log.Errorf("failed to decode process event")
		return
	}
	offset += read

	switch ProbeEventType(event.Event.Type) {
	case FileOpenEventType:
		if _, err := event.Open.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode open event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileMkdirEventType:
		if _, err := event.Mkdir.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode mkdir event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileRmdirEventType:
		if _, err := event.Rmdir.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode rmdir event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileUnlinkEventType:
		if _, err := event.Unlink.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode unlink event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileRenameEventType:
		if _, err := event.Rename.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode rename event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	default:
		log.Errorf("Unsupported event type %d\n", event.Event.Type)
	}

	log.Debugf("Dispatching event %+v\n", event)
	p.DispatchEvent(event)
}

func (p *Probe) OnNewDiscarder(event *Event, field string) error {
	log.Debugf("New discarder event %+v for field %s\n", event, field)

	eventType, err := event.GetFieldEventType(field)
	if err != nil {
		return err
	}

	filtersCb := p.eventFilterCb[eventType]
	for _, filtersCb := range filtersCb {
		value, err := event.GetFieldValue(field)
		if err != nil {
			return err
		}

		filter := eval.Filter{
			Field: field,
			Type:  eval.ScalarValueType,
			Value: value,
			Not:   true,
		}

		err = filtersCb(p, field, filter)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Probe) SetFilterPolicy(tableName string, mode PolicyMode, flags PolicyFlag) error {
	table := p.Table(tableName)
	if table == nil {
		return fmt.Errorf("unable to find policy table `%s`", tableName)
	}

	key, err := Int32ToKey(0)
	if err != nil {
		return errors.New("unable to set policy")
	}

	policy := FilterPolicy{
		Mode:  mode,
		Flags: flags,
	}
	table.Set(key, policy.Bytes())

	return nil
}

func (p *Probe) ApplyRuleSet(rs *eval.RuleSet) error {
	already := make(map[*KProbe]bool)

	for _, kprobe := range AllKProbes {
		for eventType, capabilities := range kprobe.EventTypes {
			if rs.HasRulesForEventType(eventType) {
				if _, ok := already[kprobe]; !ok {
					if err := p.Module.RegisterKprobe(kprobe.KProbe); err != nil {
						return err
					}
					already[kprobe] = true
				}

				if kprobe.PolicyTable == "" {
					continue
				}

				flags := capabilities.PolicyFlags

				if !p.enableFilters {
					log.Infof("Forcing in-kernel filter policy to `pass` for `%s`: filtering not enabled", eventType)
					if err := p.SetFilterPolicy(kprobe.PolicyTable, POLICY_MODE_ACCEPT, flags); err != nil {
						return err
					}
					continue
				}

				eventFilters, err := rs.GetEventFilters(eventType, capabilities.EvalCapabilities...)
				if err != nil || len(eventFilters) == 0 {
					log.Infof("Setting in-kernel filter policy to `pass` for `%s`: no filters", eventType)
					if err := p.SetFilterPolicy(kprobe.PolicyTable, POLICY_MODE_ACCEPT, flags); err != nil {
						return err
					}
					continue
				}

				log.Infof("Setting in-kernel filter policy to `deny` for `%s`", eventType)
				if err := p.SetFilterPolicy(kprobe.PolicyTable, POLICY_MODE_DENY, flags); err != nil {
					return err
				}

				for field, filters := range eventFilters {
					if kprobe.OnNewFilter == nil {
						continue
					}

					// if there is one not filter set the policy to ACCEPT, further filtering will
					// relies only on discarders.
					for _, filter := range filters {
						if filter.Not {
							log.Infof("Setting in-kernel filter policy to `accept` for `%s`: discarders present", eventType)
							if err := p.SetFilterPolicy(kprobe.PolicyTable, POLICY_MODE_ACCEPT, flags); err != nil {
								return err
							}
							continue
						}
					}

					if err := kprobe.OnNewFilter(p, field, filters...); err != nil {
						if err := p.SetFilterPolicy(kprobe.PolicyTable, POLICY_MODE_ACCEPT, flags); err != nil {
							return err
						}

						return err
					}
				}
			}
		}
	}

	return nil
}

func init() {
	AllKProbes = append(AllKProbes, OpenKProbes...)
}

package msgdoc

import "sort"

// Build assembles the operations collected from the DI groups into a spec.
//
// It exists mainly to own the ordering: DI resolves a group in whatever order it
// pleases, so without a sort the JSON would shuffle between runs and every
// regeneration would show a diff even when nothing changed.
func Build(operations []*OperationSpec) *MessagingSpec {
	spec := &MessagingSpec{Operations: operations}
	SortOperations(spec.Operations)
	return spec
}

// SortOperations orders operations by channel address, then action, then
// protocol.
//
// Channel first because that is how a reader looks something up — they know the
// topic, not the direction. Protocol only breaks the remaining ties, which
// happen when the same address is served over two protocols.
//
// The sort is stable, so messages declared on the same channel and action keep
// their declared order rather than being reshuffled by their names.
func SortOperations(operations []*OperationSpec) {
	sort.SliceStable(operations, func(i, j int) bool {
		a, b := operations[i], operations[j]
		if addrA, addrB := channelAddress(a), channelAddress(b); addrA != addrB {
			return addrA < addrB
		}
		if a.Action != b.Action {
			return a.Action < b.Action
		}
		return a.Protocol < b.Protocol
	})
}

// SortServers orders servers by name, for the same reason SortOperations exists.
func SortServers(servers []*ServerSpec) {
	sort.SliceStable(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})
}

// channelAddress reads an operation's address, tolerating a missing channel so
// that sorting never panics on a half-built spec.
func channelAddress(op *OperationSpec) string {
	if op == nil || op.Channel == nil {
		return ""
	}
	return op.Channel.Address
}

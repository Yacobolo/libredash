package agent

import (
	"fmt"
	"sync/atomic"
)

type IDGenerator interface {
	NewID(prefix string) string
}

type sequenceIDGenerator struct {
	n atomic.Uint64
}

func (g *sequenceIDGenerator) NewID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, g.n.Add(1))
}

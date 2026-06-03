package merit

import (
	"context"
	"testing"
	"time"
)

func TestPhase0Ref(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, taxes, _ := c.rawPost(ctx, "v1/gettaxes", "{}")
	t.Logf("TAXES:\n%s", taxes)
	_, items, _ := c.rawPost(ctx, "v1/getitems", "{}")
	if len(items) > 600 {
		items = items[:600]
	}
	t.Logf("ITEMS:\n%s", items)
}

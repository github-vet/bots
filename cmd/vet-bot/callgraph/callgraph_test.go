package callgraph_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/callgraph"
)

func TestBFSWithStack(t *testing.T) {
	tests := []struct {
		adjacencyList [][]int
		source, sink  int
		expectedPaths map[string]struct{}
	}{
		{
			[][]int{{1, 2}, {0, 3}, {0, 3}, {1, 2}}, // C_4
			0, 3,
			map[string]struct{}{
				"0 -> 1 -> 3": {},
				"0 -> 2 -> 3": {},
			},
		},
		{
			[][]int{{1, 2, 3, 4}, {0, 2, 3, 4}, {0, 1, 3, 4}, {0, 1, 2, 4}, {0, 1, 2, 3}}, // K_4
			0, 4,
			map[string]struct{}{
				"0 -> 4":      {},
				"0 -> 1 -> 4": {},
				"0 -> 2 -> 4": {},
				"0 -> 3 -> 4": {},
			},
		},
	}
	for _, test := range tests {
		graph := callgraph.NewCallGraph()
		for source, adjacent := range test.adjacencyList {
			for _, target := range adjacent {
				graph.AddCall(source, target)
			}
		}
		for source := range test.adjacencyList {
			graph.AddSignature(callgraph.Signature{"", source})
		}
		printPath := func(stack []callgraph.Signature) string {
			var sb strings.Builder
			for i, sig := range stack {
				fmt.Fprintf(&sb, "%d", sig.Arity)
				if i != len(stack)-1 {
					sb.WriteString(" -> ")
				}
			}
			return sb.String()
		}
		var paths []string
		source := callgraph.Signature{"", test.source}
		graph.BFSWithStack(source, func(signature callgraph.Signature, stack []callgraph.Signature) {
			if signature.Arity == test.sink {
				paths = append(paths, printPath(stack))
			}
		})
		matchCount := 0
		for _, path := range paths {
			if _, ok := test.expectedPaths[path]; ok {
				matchCount++
			} else {
				t.Errorf("did not find path %s", path)
			}
		}
		if matchCount != len(test.expectedPaths) {
			t.Errorf("expected %d matching paths, found only %d", len(test.expectedPaths), matchCount)
		}
	}
}

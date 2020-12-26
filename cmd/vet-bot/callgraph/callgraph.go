package callgraph

import (
	"errors"
	"sync"
)

// CallGraph represents an approximate call-graph, relying only on the name and arity of each function.
// A call-graph has a node for each function, and edges between two nodes a and b if function a calls
// function b. The call-graph this package computes is approximate in the sense that two functions with the same
// name and arity are considered equivalent. The resulting graph is a coarsening of the actual call-graph in the
// sense that two functions with matching signatures may refer to the same node in the resulting call graph. When
// this happens, the 'calls' relation described by the edges of the graph is preserved. In more complicated terms,
// when the approximate call-graph and the actual call-graph are viewed as categories, they are related by a forgetful
// functor which discards everything other than the function name and arity.
//
// The CallGraph also makes it easy to access the called-by graph, which is the callgraph with all edges reversed.
type CallGraph struct {
	signatures    []Signature
	signatureToId map[Signature]int
	callGraph     map[int][]int
	calledByGraph map[int][]int // lazily-constructed
	mut           sync.Mutex
}

func resultToCallGraph(r Result) CallGraph {
	result := NewCallGraph()
	for _, call := range r.PtrCalls {
		callerID := result.AddSignature(call.Caller.Signature)
		callID := result.AddSignature(call.Signature)
		result.AddCall(callerID, callID)
	}
	return result
}

// NewCallGraph creates an empty callgraph.
func NewCallGraph() CallGraph {
	return CallGraph{
		signatureToId: make(map[Signature]int),
		callGraph:     make(map[int][]int),
	}
}

// AddSignature adds a new signature to the callgraph and returns an ID that can
// later be used to refer to it.
func (cg *CallGraph) AddSignature(sig Signature) int {
	if id, ok := cg.signatureToId[sig]; ok {
		return id
	}
	id := len(cg.signatures)
	cg.signatures = append(cg.signatures, sig)
	cg.signatureToId[sig] = id
	return id
}

// AddCall adds an edge in the callgraph from the callerID to the callID; no attempt
// is made to check if the two signatures IDs exist in the graph. Therefore, users
// can corrupt this data structure if they are not careful.
func (cg *CallGraph) AddCall(callerID, callID int) {
	if !contains(cg.callGraph[callerID], callID) {
		cg.callGraph[callerID] = append(cg.callGraph[callerID], callID)
	}
}

// CalledByRoots returns a list of the nodes in the called-by graph which do not have any incoming edges (i.e.
// the signatures of functions which do not call any functions).
func (cg *CallGraph) CalledByRoots() []Signature {
	cg.lazyInitCalledBy()
	idSet := make(map[int]struct{})
	for id := range cg.calledByGraph {
		idSet[id] = struct{}{}
	}
	for _, callers := range cg.calledByGraph {
		for _, caller := range callers {
			delete(idSet, caller)
		}
	}
	var result []Signature
	for id := range idSet {
		result = append(result, cg.signatures[id])
	}
	return result
}

// CalledByBFS performs a breadth-first search of the called-by graph, starting from the set of roots provided.
// The provided visit function is called for every node visited during the search. Each node in the graph is visited
// at most once.
func (cg *CallGraph) CalledByBFS(roots []Signature, visit func(sig Signature)) {
	cg.lazyInitCalledBy()
	rootIDs := make([]int, 0, len(roots))
	for _, sig := range roots {
		id, ok := cg.signatureToId[sig]
		if ok {
			rootIDs = append(rootIDs, id)
		} // BFS results remain correct if nodes not in the graph remain unvisited.
	}
	frontier := make([]int, 0, len(cg.calledByGraph))
	frontier = append(frontier, rootIDs...)
	visited := make([]bool, len(cg.signatures))
	for len(frontier) > 0 {
		curr := frontier[0]
		frontier = frontier[1:]
		visited[curr] = true
		visit(cg.signatures[curr])
		for _, child := range cg.calledByGraph[curr] {
			if !visited[child] {
				frontier = append(frontier, child)
			}
		}
	}
}

// ErrSignatureMissing is returned when a request signature could not be found.
var ErrSignatureMissing error = errors.New("requested root signature does not appear in callgraph")

// BFSWithStack performs a breadth-first search of the callgraph, starting from the provided root node.
// The provided visit function is called once for every node visited during the search. Each node in the graph is
// visited once for every path from the provided root node
func (cg *CallGraph) BFSWithStack(root Signature, visit func(sig Signature, stack []Signature)) error {
	rootID, ok := cg.signatureToId[root]
	if !ok {
		return ErrSignatureMissing
	}

	type frontierNode struct {
		id    int
		stack []Signature
	}
	frontier := make([]frontierNode, 0, len(cg.callGraph))
	frontier = append(frontier, frontierNode{rootID, []Signature{root}})

	visited := make([]bool, len(cg.signatures))
	for len(frontier) > 0 {
		curr := frontier[0]
		frontier = frontier[1:]
		visited[curr.id] = true

		visit(cg.signatures[curr.id], curr.stack)
		for _, childID := range cg.callGraph[curr.id] {
			if !visited[childID] {
				newStack := make([]Signature, len(curr.stack)+1)
				copy(newStack, curr.stack)
				newStack[len(newStack)-1] = cg.signatures[childID]
				frontier = append(frontier, frontierNode{childID, newStack})
			}
		}
	}
	return nil
}

// lazyInitCalledBy initializes the calledByGraph structure if it is nil. Not all applications will require
// this graph, so it is constructed on-demand.
func (cg *CallGraph) lazyInitCalledBy() {
	cg.mut.Lock()
	defer cg.mut.Unlock()
	if cg.calledByGraph != nil {
		return
	}
	cg.calledByGraph = make(map[int][]int, len(cg.callGraph))
	for caller, callList := range cg.callGraph {
		for _, called := range callList {
			if _, ok := cg.calledByGraph[called]; ok {
				if !contains(cg.callGraph[called], caller) {
					cg.calledByGraph[called] = append(cg.calledByGraph[called], caller)
				}
			} else {
				cg.calledByGraph[called] = []int{caller}
			}
		}
	}
}

func contains(A []int, v int) bool {
	for _, a := range A {
		if a == v {
			return true
		}
	}
	return false
}

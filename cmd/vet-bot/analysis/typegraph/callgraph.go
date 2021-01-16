package typegraph

import (
	"errors"
	"go/types"
	"sync"
)

// CallGraph represents an callgraph, relying on function signatures produced during type-checking.
//
// The CallGraph also makes it easy to access the called-by graph, which is the callgraph with all edges reversed.
type CallGraph struct {
	signatures    []*types.Func
	signatureToId map[*types.Func]int
	callGraph     map[int][]int
	calledByGraph map[int][]int // lazily-constructed
	mut           sync.Mutex
}

// NewCallGraph creates an empty callgraph.
func NewCallGraph() CallGraph {
	return CallGraph{
		signatureToId: make(map[*types.Func]int),
		callGraph:     make(map[int][]int),
	}
}

// AddFunc adds a new function to the callgraph and returns an ID that can
// later be used to refer to it.
func (cg *CallGraph) AddFunc(fun *types.Func) int {
	if id, ok := cg.signatureToId[fun]; ok {
		return id
	}
	id := len(cg.signatures)
	cg.signatures = append(cg.signatures, fun)
	cg.signatureToId[fun] = id
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

// Calls returns the list of signatures called by the function with the provided ID.
func (cg *CallGraph) Calls(caller *types.Func) []*types.Func {
	id, ok := cg.signatureToId[caller]
	if !ok {
		return nil
	}
	resultIDs, ok := cg.callGraph[id]
	if !ok {
		return nil
	}
	var result []*types.Func
	for _, id := range resultIDs {
		result = append(result, cg.signatures[id])
	}
	return result
}

// CalledBy returns a list of signatures whose functions call the provided ID.
func (cg *CallGraph) CalledBy(call *types.Func) []*types.Func {
	id, ok := cg.signatureToId[call]
	if !ok {
		return nil
	}
	cg.lazyInitCalledBy()
	resultIDs, ok := cg.calledByGraph[id]
	if !ok {
		return nil
	}
	var result []*types.Func
	for _, id := range resultIDs {
		result = append(result, cg.signatures[id])
	}
	return result
}

// CalledByRoots returns a list of the nodes in the called-by graph which do not have any incoming edges (i.e.
// the signatures of functions which do not call any functions).
func (cg *CallGraph) CalledByRoots() []*types.Func {
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
	var result []*types.Func
	for id := range idSet {
		result = append(result, cg.signatures[id])
	}
	return result
}

// CalledByBFS performs a breadth-first search of the called-by graph, starting from the set of roots provided.
// The provided visit function is called for every node visited during the search. Each node in the graph is visited
// at most once.
func (cg *CallGraph) CalledByBFS(roots []*types.Func, visit func(sig *types.Func)) {
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
func (cg *CallGraph) BFSWithStack(root *types.Func, visit func(sig *types.Func, stack []*types.Func)) error {
	rootID, ok := cg.signatureToId[root]
	if !ok {
		return ErrSignatureMissing
	}

	type frontierNode struct {
		id    int
		stack []*types.Func
	}
	frontier := make([]frontierNode, 0, len(cg.callGraph))
	frontier = append(frontier, frontierNode{rootID, []*types.Func{root}})

	visited := make([]bool, len(cg.signatures))
	for len(frontier) > 0 {
		curr := frontier[0]
		frontier = frontier[1:]
		visited[curr.id] = true

		visit(cg.signatures[curr.id], curr.stack)
		for _, childID := range cg.callGraph[curr.id] {
			if !visited[childID] {
				newStack := make([]*types.Func, len(curr.stack)+1)
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

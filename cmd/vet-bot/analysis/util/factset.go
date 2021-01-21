package util

import (
	"go/types"
	"reflect"
	"sync"

	"golang.org/x/tools/go/analysis"
)

// shamelessly copied from
// https://github.com/golang/tools/blob/master/go/analysis/internal/facts/facts.go#L63-L147
type Set struct {
	pkg *types.Package
	mu  sync.Mutex
	m   map[key]analysis.Fact
}

type key struct {
	pkg *types.Package
	obj types.Object // (object facts only)
	t   reflect.Type
}

// ImportObjectFact implements analysis.Pass.ImportObjectFact.
func (s *Set) ImportObjectFact(obj types.Object, ptr analysis.Fact) bool {
	if obj == nil {
		panic("nil object")
	}
	key := key{pkg: obj.Pkg(), obj: obj, t: reflect.TypeOf(ptr)}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[key]; ok {
		reflect.ValueOf(ptr).Elem().Set(reflect.ValueOf(v).Elem())
		return true
	}
	return false
}

// ExportObjectFact implements analysis.Pass.ExportObjectFact.
func (s *Set) ExportObjectFact(obj types.Object, fact analysis.Fact) {
	key := key{pkg: obj.Pkg(), obj: obj, t: reflect.TypeOf(fact)}
	s.mu.Lock()
	s.m[key] = fact // clobber any existing entry
	s.mu.Unlock()
}

func (s *Set) AllObjectFacts() []analysis.ObjectFact {
	var facts []analysis.ObjectFact
	s.mu.Lock()
	for k, v := range s.m {
		if k.obj != nil {
			facts = append(facts, analysis.ObjectFact{Object: k.obj, Fact: v})
		}
	}
	s.mu.Unlock()
	return facts
}

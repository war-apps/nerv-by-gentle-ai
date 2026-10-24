package releaseconfig

import (
	"fmt"
	"strings"
)

// TopoOrder returns the projects in release order: every project after the
// projects it depends on, otherwise in declaration order. It fails on a
// dependency naming an unknown project and on a cycle, naming the cycle
// (e.g. "dependency cycle a -> b -> a"). Load already rejects both, so a
// loaded Release never fails here.
func (r *Release) TopoOrder() ([]Project, error) {
	index := make(map[string]int, len(r.Projects))
	for i, p := range r.Projects {
		index[p.Name] = i
	}
	const (
		unvisited = iota
		visiting
		done
	)
	state := make([]int, len(r.Projects))
	order := make([]Project, 0, len(r.Projects))
	var stack []string

	var visit func(i int) error
	visit = func(i int) error {
		p := r.Projects[i]
		switch state[i] {
		case done:
			return nil
		case visiting:
			start := 0
			for k, name := range stack {
				if name == p.Name {
					start = k
					break
				}
			}
			cycle := append(append([]string{}, stack[start:]...), p.Name)
			return fmt.Errorf("dependency cycle %s", strings.Join(cycle, " -> "))
		}
		state[i] = visiting
		stack = append(stack, p.Name)
		for _, d := range p.DependsOn {
			j, ok := index[d.Project]
			if !ok {
				return fmt.Errorf("project %q depends on unknown project %q", p.Name, d.Project)
			}
			if err := visit(j); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[i] = done
		order = append(order, p)
		return nil
	}
	for i := range r.Projects {
		if err := visit(i); err != nil {
			return nil, err
		}
	}
	return order, nil
}

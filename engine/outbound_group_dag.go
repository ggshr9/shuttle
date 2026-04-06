package engine

import "fmt"

// validateGroupDAG checks that group references form a DAG (no cycles).
// groups maps group tag → list of member tags (which may include other group tags).
func validateGroupDAG(groups map[string][]string) error {
	// DFS coloring: 0 = white (unvisited), 1 = gray (in current path), 2 = black (done)
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(groups))

	var dfs func(node string) error
	dfs = func(node string) error {
		color[node] = gray
		for _, member := range groups[node] {
			// Only recurse into nodes that are themselves groups.
			if _, isGroup := groups[member]; !isGroup {
				continue
			}
			switch color[member] {
			case gray:
				return fmt.Errorf("cycle detected: %s → %s", node, member)
			case white:
				if err := dfs(member); err != nil {
					return err
				}
			}
			// black: already fully visited, safe to skip
		}
		color[node] = black
		return nil
	}

	for node := range groups {
		if color[node] == white {
			if err := dfs(node); err != nil {
				return err
			}
		}
	}
	return nil
}

package update

import "path"

func matchGlob(pattern, name string) (bool, error) { return path.Match(pattern, name) }

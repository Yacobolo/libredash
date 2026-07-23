package report

func (targets FilterTargets) IsEmpty() bool {
	return len(targets.Visuals) == 0
}

func (targets FilterTargets) Contains(kind, id string) bool {
	return kind == "visual" && containsString(targets.Visuals, id)
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

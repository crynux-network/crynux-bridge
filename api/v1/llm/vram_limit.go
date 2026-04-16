package llm

func resolveMinVram(bodyMinVram, pathVramLimit *uint64) uint64 {
	minVram := uint64(24)
	if bodyMinVram != nil {
		minVram = *bodyMinVram
	}
	if pathVramLimit != nil {
		minVram = *pathVramLimit
	}
	return minVram
}

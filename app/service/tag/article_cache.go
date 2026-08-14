package tag

func articleTagIDValue(tagID *uint64) uint64 {
	if tagID == nil {
		return 0
	}
	return *tagID
}

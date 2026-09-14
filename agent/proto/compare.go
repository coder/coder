package proto

func LabelsEqual(a, b []*Stats_Metric_Label) bool {
	am := make(map[string]string, len(a))
	for _, lbl := range a {
		v := lbl.GetValue()
		if v == "" {
			// Prometheus considers empty labels as equivalent to being absent
			continue
		}
		am[lbl.GetName()] = lbl.GetValue()
	}
	lenB := 0
	for _, lbl := range b {
		v := lbl.GetValue()
		if v == "" {
			// Prometheus considers empty labels as equivalent to being absent
			continue
		}
		lenB++
		if am[lbl.GetName()] != v {
			return false
		}
	}
	return len(am) == lenB
}

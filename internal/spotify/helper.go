package spotify

import "fmt"

func formatDuration(totalSeconds int) string {
	h := totalSeconds / 3600
	m := (totalSeconds % 3600) / 60
	s := totalSeconds % 60

	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%d ч %d мин", h, m)
	case h > 0:
		return fmt.Sprintf("%d ч", h)
	case m > 0 && s > 0:
		return fmt.Sprintf("%d мин %d сек", m, s)
	case m > 0:
		return fmt.Sprintf("%d мин", m)
	default:
		return fmt.Sprintf("%d сек", s)
	}
}

package main

func metricReason(reason string) string {
	switch reason {
	case "dead man's switch":
		return "dms"
	case "manual (dashboard)":
		return "dashboard"
	case "manual":
		return "manual"
	default:
		return reason
	}
}

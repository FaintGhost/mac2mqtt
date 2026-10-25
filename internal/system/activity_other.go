//go:build !darwin

package system

func readActivity() Activity { return Activity{} }

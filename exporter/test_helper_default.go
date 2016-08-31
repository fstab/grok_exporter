// +build !go1.7

package exporter

import "testing"

func run(t *testing.T, name string, f func(t *testing.T)) {
	f(t)
}

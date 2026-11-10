package main

import (
	"context"
	"time"

	"mac2mqtt/internal/discover"
	"mac2mqtt/internal/service"
	"mac2mqtt/locales"
)

// findBrokers looks for brokers on the network, for the welcome to offer,
// once at a time. It asks macOS for Local Network access the first time;
// refused, it finds none.
func (a *app) findBrokers() {
	if a.searching {
		return
	}
	a.searching = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		found := discover.Find(ctx, 2*time.Second)
		a.ui(func() { a.found, a.searching = found, false })
	}()
}

// useBroker puts a broker found into Settings, which save it, where its
// user name and password go.
func (a *app) useBroker(b discover.Broker) {
	a.draft.Broker.Host = b.Host
	a.draft.Broker.TLS = b.TLS
	a.editPort = float64(b.Port)
	a.tab = tabSettings
}

// testConnection connects with the settings as edited, and says how it
// went.
func (a *app) testConnection() {
	if a.testing {
		return
	}
	cfg := a.pendingDraft()
	a.testing, a.testResult, a.testFailed = true, "", false
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := service.TestConnection(ctx, cfg, nil)
		a.ui(func() {
			a.testing = false
			if err != nil {
				a.testResult, a.testFailed = locales.TestFailed(err.Error()), true
				return
			}
			a.testResult = locales.TestSucceeded(cfg.BrokerAddress())
		})
	}()
}

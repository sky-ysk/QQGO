package middleware

import (
	"testing"
)

func TestPubSubRouterNil(t *testing.T) {
	var router *PubSubRouter
	router.Start()
	router.Stop()
	router.Subscribe("ch:qq:10001")
	router.Unsubscribe("ch:qq:10001")
	router.PublishToUser(10001, nil)
	router.PublishToGroup("G1", nil)
}

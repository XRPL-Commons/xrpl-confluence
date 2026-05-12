// Package api defines the wire types for the confluence /v1 control API.
// Every field's JSON name is part of the contract; do not rename without
// bumping APIVersion.
package api

// Version is the wire-protocol version reported by the server and CLI.
// Frozen once shipped — breaking changes go to confluence/v2.
const Version = "confluence/v1"

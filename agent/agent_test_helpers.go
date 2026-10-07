//go:build testing

package agent

// TESTING ONLY: GetConnectionManager is a helper function to get the connection manager for testing.
func (a *Agent) GetConnectionManager() *ConnectionManager {
	return a.connectionManager
}

// TESTING ONLY: GetState reads connection state with the same lock used by transitions.
func (c *ConnectionManager) GetState() ConnectionState {
	return c.getState()
}

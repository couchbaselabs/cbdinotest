package cbdino

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// AllocateOpts contains generic options for cluster allocation.
type AllocateOpts struct {
	Deployer      string
	Purpose       string
	Expiry        string
	CloudProvider string
}

// ConnstrOpts contains options for getting a connection string.
type ConnstrOpts struct {
	TLS         bool
	NoTLS       bool
	Couchbase2  bool
	DataAPI     bool
	Analytics   bool
	WaitVisible bool
}

// MgmtOpts contains options for getting the management endpoint.
type MgmtOpts struct {
	TLS   bool
	NoTLS bool
}

// BucketAddOpts contains options for adding a bucket.
type BucketAddOpts struct {
	RamQuotaMB       int
	FlushEnabled     bool
	NumReplicas      int
	IgnoreIfExisting bool
}

// UserAddOpts contains options for adding a user.
type UserAddOpts struct {
	Password string
	CanRead  bool
	CanWrite bool
}

// NodesFailoverOpts contains options for node failover.
type NodesFailoverOpts struct {
	Type        string // "hard" or "graceful"
	AllowUnsafe bool
}

// NodesSetRecoveryOpts contains options for setting node recovery.
type NodesSetRecoveryOpts struct {
	Type string // "full" or "delta"
}

// ChaosBlockTrafficOpts contains options for blocking traffic.
type ChaosBlockTrafficOpts struct {
	From       string // "nodes", "clients", or "all"
	RejectWith string
}

// ChaosPartitionTrafficOpts contains options for partitioning traffic.
type ChaosPartitionTrafficOpts struct {
	RejectWith string
}

// --- Version ---

// Version returns the cbdinocluster version string.
func (c *Controller) Version() (string, error) {
	return c.run("version")
}

// --- Allocate ---

// Allocate allocates a cluster using a simple definition tag (e.g. "simple:7.0.0").
// Returns the cluster ID.
func (c *Controller) Allocate(defTag string, opts *AllocateOpts) (string, error) {
	// TODO(brett19): In the future this should unpack the simple-def so we can do reuse.
	args := c.buildAllocateArgs(opts)
	args = append(args, defTag)
	clusterID, err := c.run(args...)
	if err != nil {
		return "", err
	}
	c.trackCluster(clusterID)
	return clusterID, nil
}

// AllocateDef allocates a cluster using an inline YAML definition string.
// Returns the cluster ID.
// If a reuse cluster ID is configured, this performs a modify instead of allocating.
func (c *Controller) AllocateDef(def string, opts *AllocateOpts) (string, error) {
	// Reuse path: modify the existing cluster to the desired state.
	if c.reuseCbdcID != nil && *c.reuseCbdcID != "" {
		existingID := *c.reuseCbdcID
		fmt.Printf("[CBDC:CTRL] reusing existing cluster %s, modifying to desired state\n", existingID)
		if err := c.Modify(existingID, def); err != nil {
			return "", fmt.Errorf("failed to modify reused cluster %s: %w", existingID, err)
		}
		fmt.Printf("[CBDC:CTRL] cluster %s modified successfully\n", existingID)
		return existingID, nil
	}

	args := c.buildAllocateArgs(opts)
	args = append(args, "--def", def)
	clusterID, err := c.run(args...)
	if err != nil {
		return "", err
	}

	if c.reuseCbdcID != nil && *c.reuseCbdcID == "" {
		*c.reuseCbdcID = clusterID
		fmt.Printf("[CBDC:CTRL] cluster %s allocated and marked for reuse (will not be removed)\n", clusterID)
	} else {
		c.trackCluster(clusterID)
	}

	return clusterID, nil
}

// AllocateDefFile allocates a cluster using a definition file path.
// Returns the cluster ID.
func (c *Controller) AllocateDefFile(defFile string, opts *AllocateOpts) (string, error) {
	// TODO(brett19): In the future this should read the file so we can do reuse.
	args := c.buildAllocateArgs(opts)
	args = append(args, "--def-file", defFile)
	clusterID, err := c.run(args...)
	if err != nil {
		return "", err
	}
	c.trackCluster(clusterID)
	return clusterID, nil
}

func (c *Controller) buildAllocateArgs(opts *AllocateOpts) []string {
	args := []string{"allocate"}
	if opts != nil {
		if opts.Deployer != "" {
			args = append(args, "--deployer", opts.Deployer)
		}
		if opts.Purpose != "" {
			args = append(args, "--purpose", opts.Purpose)
		}
		if opts.Expiry != "" {
			args = append(args, "--expiry", opts.Expiry)
		}
		if opts.CloudProvider != "" {
			args = append(args, "--cloud-provider", opts.CloudProvider)
		}
	}
	return args
}

// --- Cluster Lifecycle ---

// Remove removes a cluster by its ID.
// If the cluster matches the reuse ID, removal is skipped.
func (c *Controller) Remove(clusterID string) error {
	if c.reuseCbdcID != nil && *c.reuseCbdcID == clusterID {
		fmt.Printf("[CBDC:CTRL] skipping removal of reused cluster %s\n", clusterID)
		return nil
	}

	_, err := c.run("remove", clusterID)
	if err != nil {
		return err
	}
	c.untrackCluster(clusterID)
	return nil
}

// RemoveAll removes all clusters, optionally for a specific deployer.
func (c *Controller) RemoveAll(deployer string) error {
	args := []string{"remove-all"}
	if deployer != "" {
		args = append(args, deployer)
	}
	_, err := c.run(args...)
	return err
}

// Modify modifies a cluster using an inline JSON definition.
func (c *Controller) Modify(clusterID string, jsonDef string) error {
	_, err := c.run("modify", "--def", jsonDef, clusterID)
	return err
}

// ModifyDefFile modifies a cluster using a definition file path.
func (c *Controller) ModifyDefFile(clusterID string, defFile string) error {
	_, err := c.run("modify", "--def-file", defFile, clusterID)
	return err
}

// --- Connection Info ---

// Connstr gets the connection string for a cluster.
func (c *Controller) Connstr(clusterID string, opts *ConnstrOpts) (string, error) {
	args := []string{"connstr"}
	if opts != nil {
		if opts.TLS {
			args = append(args, "--tls")
		}
		if opts.NoTLS {
			args = append(args, "--no-tls")
		}
		if opts.Couchbase2 {
			args = append(args, "--couchbase2")
		}
		if opts.DataAPI {
			args = append(args, "--data-api")
		}
		if opts.Analytics {
			args = append(args, "--analytics")
		}
		if opts.WaitVisible {
			args = append(args, "--wait-visible")
		}
	}
	args = append(args, clusterID)
	return c.run(args...)
}

// Mgmt gets the management endpoint for a cluster.
func (c *Controller) Mgmt(clusterID string, opts *MgmtOpts) (string, error) {
	args := []string{"mgmt"}
	if opts != nil {
		if opts.TLS {
			args = append(args, "--tls")
		}
		if opts.NoTLS {
			args = append(args, "--no-tls")
		}
	}
	args = append(args, clusterID)
	return c.run(args...)
}

// IP gets the IP address of a node in a cluster.
func (c *Controller) IP(clusterID string, nodeID string) (string, error) {
	args := []string{"ip", clusterID}
	if nodeID != "" {
		args = append(args, nodeID)
	}
	return c.run(args...)
}

// --- Cluster Operations ---

// Rebalance triggers a rebalance, optionally ejecting specific nodes.
func (c *Controller) Rebalance(clusterID string, ejectNodes []string) error {
	args := []string{"rebalance", clusterID}
	args = append(args, ejectNodes...)
	_, err := c.run(args...)
	return err
}

// Query executes a N1QL query against a cluster.
func (c *Controller) Query(clusterID string, query string) (string, error) {
	return c.run("query", clusterID, query)
}

// Refresh refreshes the expiry for a cluster.
func (c *Controller) Refresh(clusterID string, expiry string) error {
	_, err := c.run("refresh", clusterID, expiry)
	return err
}

// Cleanup cleans up expired resources, optionally for a specific deployer.
func (c *Controller) Cleanup(deployer string) error {
	args := []string{"cleanup"}
	if deployer != "" {
		args = append(args, deployer)
	}
	_, err := c.run(args...)
	return err
}

// CollectLogs collects logs from a cluster into a local dest path.
func (c *Controller) CollectLogs(clusterID string, destPath string) error {
	_, err := c.run("collect-logs", clusterID, destPath)
	return err
}

// --- Buckets ---

// BucketsAdd adds a new bucket to a cluster.
func (c *Controller) BucketsAdd(clusterID, bucketName string, opts *BucketAddOpts) error {
	args := []string{"buckets", "add", clusterID, bucketName}
	if opts != nil {
		if opts.RamQuotaMB > 0 {
			args = append(args, "--ram-quota-mb", strconv.Itoa(opts.RamQuotaMB))
		}
		if opts.FlushEnabled {
			args = append(args, "--flush-enabled")
		}
		if opts.NumReplicas > 0 {
			args = append(args, "--num-replicas", strconv.Itoa(opts.NumReplicas))
		}
	}
	_, err := c.run(args...)

	// TODO(brett19): This should probably exist in cbdinocluster itself...
	if err != nil && opts != nil && opts.IgnoreIfExisting && strings.Contains(err.Error(), "already exists") {
		fmt.Printf("[CBDC:CTRL] bucket %s already exists, ignoring\n", bucketName)
		return nil
	}
	return err
}

// BucketsRemove removes a bucket from a cluster.
func (c *Controller) BucketsRemove(clusterID, bucketName string) error {
	_, err := c.run("buckets", "remove", clusterID, bucketName)
	return err
}

// BucketsLoadSample loads a sample bucket on a cluster.
func (c *Controller) BucketsLoadSample(clusterID, sampleName string) error {
	_, err := c.run("buckets", "load-sample", clusterID, sampleName)
	return err
}

// --- Users ---

// UsersAdd adds a user to a cluster.
func (c *Controller) UsersAdd(clusterID, username string, opts *UserAddOpts) error {
	args := []string{"users", "add", clusterID, username}
	if opts != nil {
		if opts.Password != "" {
			args = append(args, "--password", opts.Password)
		}
		if !opts.CanRead {
			args = append(args, "--can-read=false")
		}
		if !opts.CanWrite {
			args = append(args, "--can-write=false")
		}
	}
	_, err := c.run(args...)
	return err
}

// UsersRemove removes a user from a cluster.
func (c *Controller) UsersRemove(clusterID, username string) error {
	_, err := c.run("users", "remove", clusterID, username)
	return err
}

// --- Collections ---

// CollectionsAddScope adds a scope to a bucket.
func (c *Controller) CollectionsAddScope(clusterID, bucketName, scopeName string) error {
	_, err := c.run("collections", "add-scope", clusterID, bucketName, scopeName)
	return err
}

// CollectionsAdd adds a collection to a scope.
func (c *Controller) CollectionsAdd(clusterID, bucketName, scopeName, collectionName string) error {
	_, err := c.run("collections", "add", clusterID, bucketName, scopeName, collectionName)
	return err
}

// CollectionsRemoveScope removes a scope from a bucket.
func (c *Controller) CollectionsRemoveScope(clusterID, bucketName, scopeName string) error {
	_, err := c.run("collections", "remove-scope", clusterID, bucketName, scopeName)
	return err
}

// CollectionsRemove removes a collection from a scope.
func (c *Controller) CollectionsRemove(clusterID, bucketName, scopeName, collectionName string) error {
	_, err := c.run("collections", "remove", clusterID, bucketName, scopeName, collectionName)
	return err
}

// --- Nodes ---

// NodesAdd adds a new node to the cluster. Returns the node ID.
func (c *Controller) NodesAdd(clusterID string) (string, error) {
	return c.run("nodes", "add", clusterID)
}

// NodesRemove removes a node from the cluster.
func (c *Controller) NodesRemove(clusterID, nodeID string) error {
	_, err := c.run("nodes", "remove", clusterID, nodeID)
	return err
}

// NodesFailover fails over a node in the cluster.
func (c *Controller) NodesFailover(clusterID, nodeID string, opts *NodesFailoverOpts) error {
	args := []string{"nodes", "failover", clusterID, nodeID}
	if opts != nil {
		if opts.Type != "" {
			args = append(args, "--type", opts.Type)
		}
		if opts.AllowUnsafe {
			args = append(args, "--allow-unsafe")
		}
	}
	_, err := c.run(args...)
	return err
}

// NodesSetRecovery sets recovery type for a node in the cluster.
func (c *Controller) NodesSetRecovery(clusterID, nodeID string, opts *NodesSetRecoveryOpts) error {
	args := []string{"nodes", "set-recovery", clusterID, nodeID}
	if opts != nil {
		if opts.Type != "" {
			args = append(args, "--type", opts.Type)
		}
	}
	_, err := c.run(args...)
	return err
}

// --- Chaos ---

// ChaosBlockTraffic blocks traffic to specific nodes.
func (c *Controller) ChaosBlockTraffic(clusterID string, nodeIDs []string, opts *ChaosBlockTrafficOpts) error {
	args := []string{"chaos", "block-traffic"}
	if opts != nil {
		if opts.From != "" {
			args = append(args, "--from", opts.From)
		}
		if opts.RejectWith != "" {
			args = append(args, "--reject-with", opts.RejectWith)
		}
	}
	args = append(args, clusterID)
	args = append(args, nodeIDs...)
	_, err := c.run(args...)
	return err
}

// ChaosAllowTraffic allows all traffic to specific nodes.
func (c *Controller) ChaosAllowTraffic(clusterID string, nodeIDs []string) error {
	args := []string{"chaos", "allow-traffic", clusterID}
	args = append(args, nodeIDs...)
	_, err := c.run(args...)
	return err
}

// ChaosKillCouchbase kills the Couchbase service on specific nodes.
func (c *Controller) ChaosKillCouchbase(clusterID string, nodeIDs []string) error {
	args := []string{"chaos", "kill-couchbase", clusterID}
	args = append(args, nodeIDs...)
	_, err := c.run(args...)
	return err
}

// ChaosPartitionTraffic partitions intra-node traffic in the cluster.
func (c *Controller) ChaosPartitionTraffic(clusterID string, nodeIDs []string, opts *ChaosPartitionTrafficOpts) error {
	args := []string{"chaos", "partition-traffic"}
	if opts != nil {
		if opts.RejectWith != "" {
			args = append(args, "--reject-with", opts.RejectWith)
		}
	}
	args = append(args, clusterID)
	args = append(args, nodeIDs...)
	_, err := c.run(args...)
	return err
}

// ChaosPickNodeOpts contains options for picking a node.
type ChaosPickNodeOpts struct {
	Orchestrator bool // Select the orchestrator instead of a non-orchestrator
}

// ChaosPickNode picks a single node from the cluster for chaos testing.
// By default it excludes the orchestrator; use Orchestrator option to select it instead.
func (c *Controller) ChaosPickNode(clusterID string, opts *ChaosPickNodeOpts) (string, error) {
	args := []string{"chaos", "pick-node"}
	if opts != nil {
		if opts.Orchestrator {
			args = append(args, "--orchestrator")
		}
	}
	args = append(args, clusterID)
	return c.run(args...)
}

// ChaosPauseNode pauses specific nodes in the cluster.
func (c *Controller) ChaosPauseNode(clusterID string, nodeIDs []string) error {
	args := []string{"chaos", "pause-node", clusterID}
	args = append(args, nodeIDs...)
	_, err := c.run(args...)
	return err
}

// ChaosUnpauseNode unpauses specific nodes in the cluster.
func (c *Controller) ChaosUnpauseNode(clusterID string, nodeIDs []string) error {
	args := []string{"chaos", "unpause-node", clusterID}
	args = append(args, nodeIDs...)
	_, err := c.run(args...)
	return err
}

// ClusterSettingsEnableAutoFailover enables auto-failover for a cluster with an optional timeout.
func (c *Controller) ClusterSettingsEnableAutoFailover(clusterID string, timeout int) error {
	args := []string{"cluster-settings", "enable-autofailover"}
	if timeout > 0 {
		args = append(args, "--timeout", strconv.Itoa(timeout))
	}
	args = append(args, clusterID)
	_, err := c.run(args...)
	return err
}

// ClusterSettingsDisableAutoFailover disables auto-failover for a cluster.
func (c *Controller) ClusterSettingsDisableAutoFailover(clusterID string) error {
	args := []string{"cluster-settings", "disable-autofailover", clusterID}
	_, err := c.run(args...)
	return err
}

// --- Certificates ---

// CertificatesGetCA fetches the CA certificate for a cluster.
func (c *Controller) CertificatesGetCA(clusterID string) (string, error) {
	return c.run("certificates", "get-ca", clusterID)
}

// --- Helpers ---

// AllocateOptsToJSON converts a JS-style allocate options map to a JSON definition string.
func AllocateOptsToJSON(opts map[string]interface{}) (string, error) {
	data, err := json.Marshal(opts)
	if err != nil {
		return "", fmt.Errorf("failed to marshal allocate options: %w", err)
	}
	return string(data), nil
}

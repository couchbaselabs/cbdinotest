package jsexec

import (
	"encoding/json"
	"fmt"

	"github.com/couchbaselabs/cbdinotest/cbdino"
	"github.com/dop251/goja"
)

// CbdtCbdinoShim is the JS shim that esbuild injects for "cbdt:cbdino".
const CbdtCbdinoShim = `export default __cbdt_cbdino;`

// cbdinoCluster represents an allocated cluster with a unique identifier.
type cbdinoCluster struct {
	vm        *Runtime
	ctrl      *cbdino.Controller
	clusterID string
}

func (c *cbdinoCluster) id(call goja.FunctionCall) goja.Value {
	return c.vm.ToValue(c.clusterID)
}

func (c *cbdinoCluster) toString(call goja.FunctionCall) goja.Value {
	return c.vm.ToValue(fmt.Sprintf("[Cluster %s]", c.clusterID))
}

func (c *cbdinoCluster) connstr(call goja.FunctionCall) goja.Value {
	opts := &cbdino.ConnstrOpts{}
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
		optsObj := call.Argument(0).ToObject(c.vm.Runtime)
		if v := optsObj.Get("tls"); v != nil && !goja.IsUndefined(v) {
			opts.TLS = v.ToBoolean()
		}
		if v := optsObj.Get("noTLS"); v != nil && !goja.IsUndefined(v) {
			opts.NoTLS = v.ToBoolean()
		}
		if v := optsObj.Get("couchbase2"); v != nil && !goja.IsUndefined(v) {
			opts.Couchbase2 = v.ToBoolean()
		}
		if v := optsObj.Get("dataAPI"); v != nil && !goja.IsUndefined(v) {
			opts.DataAPI = v.ToBoolean()
		}
		if v := optsObj.Get("analytics"); v != nil && !goja.IsUndefined(v) {
			opts.Analytics = v.ToBoolean()
		}
		if v := optsObj.Get("waitVisible"); v != nil && !goja.IsUndefined(v) {
			opts.WaitVisible = v.ToBoolean()
		}
	}

	fmt.Printf("[MOD:CBDINO] getting connstr for cluster %s\n", c.clusterID)

	connStr, err := c.ctrl.Connstr(c.clusterID, opts)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to get connstr: %w", err)))
	}

	return c.vm.ToValue(connStr)
}

func (c *cbdinoCluster) modify(call goja.FunctionCall) goja.Value {
	defArg := call.Argument(0)
	if defArg == nil || goja.IsUndefined(defArg) {
		panic(c.vm.NewGoError(fmt.Errorf("modify requires a definition argument")))
	}

	jsonDef, err := jsonFromValue(defArg)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to marshal modify definition: %w", err)))
	}

	fmt.Printf("[MOD:CBDINO] modifying cluster %s with def: %s\n", c.clusterID, jsonDef)

	err = c.ctrl.Modify(c.clusterID, jsonDef)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to modify cluster: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) remove(call goja.FunctionCall) goja.Value {
	fmt.Printf("[MOD:CBDINO] removing cluster %s\n", c.clusterID)

	err := c.ctrl.Remove(c.clusterID)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to remove cluster: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) mgmt(call goja.FunctionCall) goja.Value {
	fmt.Printf("[MOD:CBDINO] getting mgmt endpoint for cluster %s\n", c.clusterID)

	mgmtUri, err := c.ctrl.Mgmt(c.clusterID, nil)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to get mgmt endpoint: %w", err)))
	}

	return c.vm.ToValue(mgmtUri)
}

func (c *cbdinoCluster) rebalance(call goja.FunctionCall) goja.Value {
	var ejectNodes []string
	for _, arg := range call.Arguments {
		ejectNodes = append(ejectNodes, arg.String())
	}

	fmt.Printf("[MOD:CBDINO] rebalancing cluster %s, ejecting nodes: %v\n", c.clusterID, ejectNodes)

	err := c.ctrl.Rebalance(c.clusterID, ejectNodes)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to rebalance: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) query(call goja.FunctionCall) goja.Value {
	queryStr := call.Argument(0).String()

	fmt.Printf("[MOD:CBDINO] executing query on cluster %s: %s\n", c.clusterID, queryStr)

	result, err := c.ctrl.Query(c.clusterID, queryStr)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to execute query: %w", err)))
	}

	return c.vm.ToValue(result)
}

// --- Bucket methods ---

func (c *cbdinoCluster) bucketsAdd(call goja.FunctionCall) goja.Value {
	bucketName := call.Argument(0).String()

	opts := &cbdino.BucketAddOpts{}
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
		optsObj := call.Argument(1).ToObject(c.vm.Runtime)
		if v := optsObj.Get("ramQuotaMB"); v != nil && !goja.IsUndefined(v) {
			opts.RamQuotaMB = int(v.ToInteger())
		}
		if v := optsObj.Get("flushEnabled"); v != nil && !goja.IsUndefined(v) {
			opts.FlushEnabled = v.ToBoolean()
		}
		if v := optsObj.Get("numReplicas"); v != nil && !goja.IsUndefined(v) {
			opts.NumReplicas = int(v.ToInteger())
		}
		if v := optsObj.Get("ignoreIfExisting"); v != nil && !goja.IsUndefined(v) {
			opts.IgnoreIfExisting = v.ToBoolean()
		}
	}

	fmt.Printf("[MOD:CBDINO] adding bucket %s to cluster %s (ramQuotaMB=%d, flushEnabled=%t, numReplicas=%d)\n", bucketName, c.clusterID, opts.RamQuotaMB, opts.FlushEnabled, opts.NumReplicas)

	err := c.ctrl.BucketsAdd(c.clusterID, bucketName, opts)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to add bucket: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) bucketsRemove(call goja.FunctionCall) goja.Value {
	bucketName := call.Argument(0).String()

	fmt.Printf("[MOD:CBDINO] removing bucket %s from cluster %s\n", bucketName, c.clusterID)

	err := c.ctrl.BucketsRemove(c.clusterID, bucketName)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to remove bucket: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) bucketsLoadSample(call goja.FunctionCall) goja.Value {
	sampleName := call.Argument(0).String()

	fmt.Printf("[MOD:CBDINO] loading sample bucket %s on cluster %s\n", sampleName, c.clusterID)

	err := c.ctrl.BucketsLoadSample(c.clusterID, sampleName)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to load sample bucket: %w", err)))
	}

	return goja.Undefined()
}

// --- User methods ---

func (c *cbdinoCluster) usersAdd(call goja.FunctionCall) goja.Value {
	username := call.Argument(0).String()

	opts := &cbdino.UserAddOpts{
		CanRead:  true,
		CanWrite: true,
	}
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
		optsObj := call.Argument(1).ToObject(c.vm.Runtime)
		if v := optsObj.Get("password"); v != nil && !goja.IsUndefined(v) {
			opts.Password = v.String()
		}
		if v := optsObj.Get("canRead"); v != nil && !goja.IsUndefined(v) {
			opts.CanRead = v.ToBoolean()
		}
		if v := optsObj.Get("canWrite"); v != nil && !goja.IsUndefined(v) {
			opts.CanWrite = v.ToBoolean()
		}
	}

	fmt.Printf("[MOD:CBDINO] adding user %s to cluster %s (canRead=%t, canWrite=%t)\n", username, c.clusterID, opts.CanRead, opts.CanWrite)

	err := c.ctrl.UsersAdd(c.clusterID, username, opts)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to add user: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) usersRemove(call goja.FunctionCall) goja.Value {
	username := call.Argument(0).String()

	fmt.Printf("[MOD:CBDINO] removing user %s from cluster %s\n", username, c.clusterID)

	err := c.ctrl.UsersRemove(c.clusterID, username)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to remove user: %w", err)))
	}

	return goja.Undefined()
}

// --- Collection methods ---

func (c *cbdinoCluster) collectionsAddScope(call goja.FunctionCall) goja.Value {
	bucketName := call.Argument(0).String()
	scopeName := call.Argument(1).String()

	fmt.Printf("[MOD:CBDINO] adding scope %s/%s on cluster %s\n", bucketName, scopeName, c.clusterID)

	err := c.ctrl.CollectionsAddScope(c.clusterID, bucketName, scopeName)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to add scope: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) collectionsAdd(call goja.FunctionCall) goja.Value {
	bucketName := call.Argument(0).String()
	scopeName := call.Argument(1).String()
	collectionName := call.Argument(2).String()

	fmt.Printf("[MOD:CBDINO] adding collection %s/%s/%s on cluster %s\n", bucketName, scopeName, collectionName, c.clusterID)

	err := c.ctrl.CollectionsAdd(c.clusterID, bucketName, scopeName, collectionName)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to add collection: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) collectionsRemoveScope(call goja.FunctionCall) goja.Value {
	bucketName := call.Argument(0).String()
	scopeName := call.Argument(1).String()

	fmt.Printf("[MOD:CBDINO] removing scope %s/%s on cluster %s\n", bucketName, scopeName, c.clusterID)

	err := c.ctrl.CollectionsRemoveScope(c.clusterID, bucketName, scopeName)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to remove scope: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) collectionsRemove(call goja.FunctionCall) goja.Value {
	bucketName := call.Argument(0).String()
	scopeName := call.Argument(1).String()
	collectionName := call.Argument(2).String()

	fmt.Printf("[MOD:CBDINO] removing collection %s/%s/%s on cluster %s\n", bucketName, scopeName, collectionName, c.clusterID)

	err := c.ctrl.CollectionsRemove(c.clusterID, bucketName, scopeName, collectionName)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to remove collection: %w", err)))
	}

	return goja.Undefined()
}

// --- Node methods ---

func (c *cbdinoCluster) nodesAdd(call goja.FunctionCall) goja.Value {
	fmt.Printf("[MOD:CBDINO] adding node to cluster %s\n", c.clusterID)

	nodeID, err := c.ctrl.NodesAdd(c.clusterID)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to add node: %w", err)))
	}

	return c.vm.ToValue(nodeID)
}

func (c *cbdinoCluster) nodesRemove(call goja.FunctionCall) goja.Value {
	nodeID := call.Argument(0).String()

	fmt.Printf("[MOD:CBDINO] removing node %s from cluster %s\n", nodeID, c.clusterID)

	err := c.ctrl.NodesRemove(c.clusterID, nodeID)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to remove node: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) nodesFailover(call goja.FunctionCall) goja.Value {
	nodeID := call.Argument(0).String()
	opts := &cbdino.NodesFailoverOpts{Type: "hard"}

	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
		optsObj := call.Argument(1).ToObject(c.vm.Runtime)
		if v := optsObj.Get("type"); v != nil && !goja.IsUndefined(v) {
			opts.Type = v.String()
		}
		if v := optsObj.Get("allowUnsafe"); v != nil && !goja.IsUndefined(v) {
			opts.AllowUnsafe = v.ToBoolean()
		}
	}

	fmt.Printf("[MOD:CBDINO] failing over node %s on cluster %s (type=%s, allowUnsafe=%t)\n", nodeID, c.clusterID, opts.Type, opts.AllowUnsafe)

	err := c.ctrl.NodesFailover(c.clusterID, nodeID, opts)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to failover node: %w", err)))
	}

	return goja.Undefined()
}

// --- Chaos methods ---

func (c *cbdinoCluster) chaosBlockTraffic(call goja.FunctionCall) goja.Value {
	nodeIDs := ExtractStringArray(c.vm, call.Argument(0))
	opts := &cbdino.ChaosBlockTrafficOpts{From: "nodes"}
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
		optsObj := call.Argument(1).ToObject(c.vm.Runtime)
		if v := optsObj.Get("from"); v != nil && !goja.IsUndefined(v) {
			opts.From = v.String()
		}
	}

	fmt.Printf("[MOD:CBDINO] blocking traffic on cluster %s, nodes=%v, from=%s\n", c.clusterID, nodeIDs, opts.From)

	err := c.ctrl.ChaosBlockTraffic(c.clusterID, nodeIDs, opts)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to block traffic: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) chaosAllowTraffic(call goja.FunctionCall) goja.Value {
	nodeIDs := ExtractStringArray(c.vm, call.Argument(0))

	fmt.Printf("[MOD:CBDINO] allowing traffic on cluster %s, nodes=%v\n", c.clusterID, nodeIDs)

	err := c.ctrl.ChaosAllowTraffic(c.clusterID, nodeIDs)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to allow traffic: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) chaosKillCouchbase(call goja.FunctionCall) goja.Value {
	nodeIDs := ExtractStringArray(c.vm, call.Argument(0))

	fmt.Printf("[MOD:CBDINO] killing couchbase on cluster %s, nodes=%v\n", c.clusterID, nodeIDs)

	err := c.ctrl.ChaosKillCouchbase(c.clusterID, nodeIDs)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to kill couchbase: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) chaosPauseNode(call goja.FunctionCall) goja.Value {
	nodeIDs := ExtractStringArray(c.vm, call.Argument(0))

	fmt.Printf("[MOD:CBDINO] pausing nodes on cluster %s, nodes=%v\n", c.clusterID, nodeIDs)

	err := c.ctrl.ChaosPauseNode(c.clusterID, nodeIDs)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to pause nodes: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) chaosUnpauseNode(call goja.FunctionCall) goja.Value {
	nodeIDs := ExtractStringArray(c.vm, call.Argument(0))

	fmt.Printf("[MOD:CBDINO] unpausing nodes on cluster %s, nodes=%v\n", c.clusterID, nodeIDs)

	err := c.ctrl.ChaosUnpauseNode(c.clusterID, nodeIDs)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to unpause nodes: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) chaosPickNode(call goja.FunctionCall) goja.Value {
	opts := &cbdino.ChaosPickNodeOpts{}
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
		optsObj := call.Argument(0).ToObject(c.vm.Runtime)
		if v := optsObj.Get("orchestrator"); v != nil && !goja.IsUndefined(v) {
			opts.Orchestrator = v.ToBoolean()
		}
	}

	fmt.Printf("[MOD:CBDINO] picking node on cluster %s (orchestrator=%t)\n", c.clusterID, opts.Orchestrator)

	nodeID, err := c.ctrl.ChaosPickNode(c.clusterID, opts)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to pick node: %w", err)))
	}

	return c.vm.ToValue(nodeID)
}

func (c *cbdinoCluster) clusterSettingsEnableAutoFailover(call goja.FunctionCall) goja.Value {
	timeout := 0
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
		optsObj := call.Argument(0).ToObject(c.vm.Runtime)
		if v := optsObj.Get("timeout"); v != nil && !goja.IsUndefined(v) {
			timeout = int(v.ToInteger())
		}
	}

	fmt.Printf("[MOD:CBDINO] enabling auto-failover on cluster %s (timeout=%d)\n", c.clusterID, timeout)

	err := c.ctrl.ClusterSettingsEnableAutoFailover(c.clusterID, timeout)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to enable auto-failover: %w", err)))
	}

	return goja.Undefined()
}

func (c *cbdinoCluster) clusterSettingsDisableAutoFailover(call goja.FunctionCall) goja.Value {
	fmt.Printf("[MOD:CBDINO] disabling auto-failover on cluster %s\n", c.clusterID)

	err := c.ctrl.ClusterSettingsDisableAutoFailover(c.clusterID)
	if err != nil {
		panic(c.vm.NewGoError(fmt.Errorf("failed to disable auto-failover: %w", err)))
	}

	return goja.Undefined()
}

// newClusterObject creates a Goja object representing a cbdinoCluster
// with all methods wired up.
func newClusterObject(vm *Runtime, cluster *cbdinoCluster) *goja.Object {
	obj := vm.NewObject()

	obj.Set("id", cluster.id)
	obj.Set("toString", cluster.toString)
	obj.Set("connstr", cluster.connstr)

	obj.Set("modify", cluster.modify)
	obj.Set("remove", cluster.remove)
	obj.Set("mgmt", cluster.mgmt)
	obj.Set("rebalance", cluster.rebalance)
	obj.Set("query", cluster.query)

	buckets := vm.NewObject()
	buckets.Set("add", cluster.bucketsAdd)
	buckets.Set("remove", cluster.bucketsRemove)
	buckets.Set("loadSample", cluster.bucketsLoadSample)
	obj.Set("buckets", buckets)

	users := vm.NewObject()
	users.Set("add", cluster.usersAdd)
	users.Set("remove", cluster.usersRemove)
	obj.Set("users", users)

	collections := vm.NewObject()
	collections.Set("add", cluster.collectionsAdd)
	collections.Set("addScope", cluster.collectionsAddScope)
	collections.Set("remove", cluster.collectionsRemove)
	collections.Set("removeScope", cluster.collectionsRemoveScope)
	obj.Set("collections", collections)

	nodes := vm.NewObject()
	nodes.Set("add", cluster.nodesAdd)
	nodes.Set("remove", cluster.nodesRemove)
	nodes.Set("failover", cluster.nodesFailover)
	obj.Set("nodes", nodes)

	chaos := vm.NewObject()
	chaos.Set("blockTraffic", cluster.chaosBlockTraffic)
	chaos.Set("allowTraffic", cluster.chaosAllowTraffic)
	chaos.Set("killCouchbase", cluster.chaosKillCouchbase)
	chaos.Set("pauseNode", cluster.chaosPauseNode)
	chaos.Set("unpauseNode", cluster.chaosUnpauseNode)
	chaos.Set("pickNode", cluster.chaosPickNode)
	obj.Set("chaos", chaos)

	settings := vm.NewObject()
	settings.Set("enableAutoFailover", cluster.clusterSettingsEnableAutoFailover)
	settings.Set("disableAutoFailover", cluster.clusterSettingsDisableAutoFailover)
	obj.Set("settings", settings)

	return obj
}

// SetupCbdtCbdino registers the cbdt:cbdino module object on the Goja runtime.
// SetupCbdtCbdino registers the cbdt:cbdino module object on the runtime.
func SetupCbdtCbdino(vm *Runtime, ctrl *cbdino.Controller) {
	cbdinoObj := vm.NewObject()
	cbdinoObj.Set("allocate", cbdinoAllocate(vm, ctrl))
	cbdinoObj.Set("removeAll", cbdinoRemoveAll(vm, ctrl))
	cbdinoObj.Set("cleanup", cbdinoCleanup(vm, ctrl))
	vm.Set("__cbdt_cbdino", cbdinoObj)
}

func cbdinoAllocate(vm *Runtime, ctrl *cbdino.Controller) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		defArg := call.Argument(0)
		if defArg == nil || goja.IsUndefined(defArg) {
			panic(vm.NewGoError(fmt.Errorf("allocate requires a definition argument")))
		}

		var clusterID string
		var err error

		if defArg.ExportType().Kind().String() == "string" {
			defTag := defArg.String()
			fmt.Printf("[MOD:CBDINO] allocating cluster with def tag: %s\n", defTag)
			clusterID, err = ctrl.Allocate(defTag, nil)
		} else {
			jsonDef, jsonErr := jsonFromValue(defArg)
			if jsonErr != nil {
				panic(vm.NewGoError(fmt.Errorf("failed to marshal allocate definition: %w", jsonErr)))
			}
			fmt.Printf("[MOD:CBDINO] allocating cluster with YAML def\n")
			clusterID, err = ctrl.AllocateDef(jsonDef, nil)
		}

		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("failed to allocate cluster: %w", err)))
		}

		fmt.Printf("[MOD:CBDINO] allocated cluster %s\n", clusterID)

		cluster := &cbdinoCluster{
			vm:        vm,
			ctrl:      ctrl,
			clusterID: clusterID,
		}
		return newClusterObject(vm, cluster)
	}
}

func cbdinoRemoveAll(vm *Runtime, ctrl *cbdino.Controller) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		deployer := ""
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			deployer = call.Argument(0).String()
		}

		fmt.Printf("[MOD:CBDINO] removing all clusters (deployer=%q)\n", deployer)

		err := ctrl.RemoveAll(deployer)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("failed to remove all clusters: %w", err)))
		}

		return goja.Undefined()
	}
}

func cbdinoCleanup(vm *Runtime, ctrl *cbdino.Controller) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		deployer := ""
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			deployer = call.Argument(0).String()
		}

		fmt.Printf("[MOD:CBDINO] cleaning up expired resources (deployer=%q)\n", deployer)

		err := ctrl.Cleanup(deployer)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("failed to cleanup: %w", err)))
		}

		return goja.Undefined()
	}
}

// --- Helpers ---

// jsonFromValue converts a goja Value to a JSON string.
func jsonFromValue(val goja.Value) (string, error) {
	exported := val.Export()
	data, err := json.Marshal(exported)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ExtractStringArray extracts a JS array of strings from a goja Value.
func ExtractStringArray(vm *Runtime, val goja.Value) []string {
	if val == nil || goja.IsUndefined(val) {
		return nil
	}

	obj := val.ToObject(vm.Runtime)
	var result []string

	lengthVal := obj.Get("length")
	if lengthVal != nil && !goja.IsUndefined(lengthVal) {
		length := int(lengthVal.ToInteger())
		for i := 0; i < length; i++ {
			elem := obj.Get(fmt.Sprintf("%d", i))
			if elem != nil && !goja.IsUndefined(elem) {
				result = append(result, elem.String())
			}
		}
	}

	return result
}

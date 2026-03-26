// Type definitions for the cbdt:cbdino module.
// Provides cluster allocation and management via cbdinocluster.

/** Options for allocating a cluster with an inline definition. */
interface AllocateNodeDef {
  /** Number of nodes with this configuration. */
  count: number;
  /** Server version string (e.g. "7.6.0"). */
  version: string;
  /** Services to enable on these nodes. */
  services: Array<"kv" | "n1ql" | "index" | "fts" | "cbas" | "eventing" | "backup">;
  /** Force new nodes to be provisioned instead of reusing existing ones during modifications. */
  "force-new"?: boolean;
}

/** Inline cluster definition passed to `cbdino.allocate()`. */
interface AllocateDef {
  /** Node groups for the cluster. */
  nodes: AllocateNodeDef[];
  /** Deployer to use (e.g. "docker", "cloud", "cao"). */
  deployer?: string;
}

/** Options for adding a bucket. */
interface BucketAddOpts {
  /** RAM quota in MB. */
  ramQuotaMB?: number;
  /** Whether flush is enabled on the bucket. */
  flushEnabled?: boolean;
  /** Number of replicas for the bucket. */
  numReplicas?: number;
  /** If true, silently succeed when the bucket already exists. */
  ignoreIfExisting?: boolean;
}

/** Options for adding a user. */
interface UserAddOpts {
  /** User password. */
  password?: string;
  /** Whether the user has read access (defaults to true). */
  canRead?: boolean;
  /** Whether the user has write access (defaults to true). */
  canWrite?: boolean;
}

/** Options for node failover. */
interface NodesFailoverOpts {
  /** Failover type: "hard" or "graceful". Defaults to "hard". */
  type?: "hard" | "graceful";
  /** Whether to allow unsafe failover. */
  allowUnsafe?: boolean;
}

/** Options for picking a node for chaos testing. */
interface ChaosPickNodeOpts {
  /** If true, select the orchestrator node instead of a non-orchestrator. */
  orchestrator?: boolean;
}

/** Options for getting a connection string. */
interface ConnstrOpts {
  /** Explicitly request a TLS endpoint. */
  tls?: boolean;
  /** Explicitly request a non-TLS endpoint. */
  noTLS?: boolean;
  /** Request a couchbase2:// connection string. */
  couchbase2?: boolean;
  /** Request a Data API connection string. */
  dataAPI?: boolean;
  /** Request an Analytics connection string. */
  analytics?: boolean;
  /** Wait for DNS to be visible to this host. */
  waitVisible?: boolean;
}

/** Bucket management sub-object on a Cluster. */
interface ClusterBuckets {
  /** Add a bucket to the cluster. */
  add(bucketName: string, opts?: BucketAddOpts): void;
  /** Remove a bucket from the cluster. */
  remove(bucketName: string): void;
  /** Load a sample bucket (e.g. "travel-sample", "beer-sample"). */
  loadSample(sampleName: string): void;
}

/** User management sub-object on a Cluster. */
interface ClusterUsers {
  /** Add a user to the cluster. */
  add(username: string, opts?: UserAddOpts): void;
  /** Remove a user from the cluster. */
  remove(username: string): void;
}

/** Collection management sub-object on a Cluster. */
interface ClusterCollections {
  /** Add a collection to a scope. */
  add(bucketName: string, scopeName: string, collectionName: string): void;
  /** Add a scope to a bucket. */
  addScope(bucketName: string, scopeName: string): void;
  /** Remove a collection from a scope. */
  remove(bucketName: string, scopeName: string, collectionName: string): void;
  /** Remove a scope from a bucket. */
  removeScope(bucketName: string, scopeName: string): void;
}

/** Node management sub-object on a Cluster. */
interface ClusterNodes {
  /** Add a new node to the cluster. Returns the node ID. */
  add(): string;
  /** Remove a node from the cluster. */
  remove(nodeID: string): void;
  /** Failover a node. */
  failover(nodeID: string, opts?: NodesFailoverOpts): void;
}

/** Chaos engineering sub-object on a Cluster. */
interface ClusterChaos {
  /**
   * Block traffic to/from specific nodes.
   * @param nodeIDs - Array of node IDs to block.
   * @param from - Traffic direction: "nodes", "clients", or "all". Defaults to "nodes".
   */
  blockTraffic(nodeIDs: string[], from?: string): void;
  /** Allow all traffic to the specified nodes. */
  allowTraffic(nodeIDs: string[]): void;
  /** Kill the Couchbase service on specific nodes. */
  killCouchbase(nodeIDs: string[]): void;
  /** Pause specific nodes (docker pause). */
  pauseNode(nodeIDs: string[]): void;
  /** Unpause specific nodes (docker unpause). */
  unpauseNode(nodeIDs: string[]): void;
  /**
   * Pick a single node from the cluster for chaos testing.
   * By default excludes the orchestrator.
   * @param opts - Options to control node selection.
   * @returns The node ID.
   */
  pickNode(opts?: ChaosPickNodeOpts): string;
}

/** Represents an allocated Couchbase cluster. */
interface Cluster {
  /** Returns the cluster UUID. */
  id(): string;
  /** Returns a string representation of the cluster. */
  toString(): string;
  /** Returns a connection string for the cluster. */
  connstr(opts?: ConnstrOpts): string;
  /** Modify the cluster topology using an inline definition. */
  modify(def: AllocateDef): void;
  /** Remove (deallocate) the cluster. */
  remove(): void;
  /** Returns the management endpoint URI. */
  mgmt(): string;
  /** Trigger a rebalance, optionally ejecting specific nodes. */
  rebalance(...ejectNodeIDs: string[]): void;
  /** Execute a N1QL query on the cluster. */
  query(statement: string): string;

  /** Bucket management operations. */
  readonly buckets: ClusterBuckets;
  /** User management operations. */
  readonly users: ClusterUsers;
  /** Collection/scope management operations. */
  readonly collections: ClusterCollections;
  /** Node management operations. */
  readonly nodes: ClusterNodes;
  /** Chaos engineering operations. */
  readonly chaos: ClusterChaos;
}

/** The cbdt:cbdino module. */
interface CbdinoModule {
  /**
   * Allocate a new cluster.
   * Pass a simple definition tag string (e.g. "simple:7.6.0") or
   * an inline definition object.
   */
  allocate(def: string | AllocateDef): Cluster;
  /** Remove all clusters, optionally filtering by deployer. */
  removeAll(deployer?: string): void;
  /** Cleanup expired resources, optionally filtering by deployer. */
  cleanup(deployer?: string): void;
}

declare module "cbdt:cbdino" {
  const cbdino: CbdinoModule;
  export default cbdino;
}

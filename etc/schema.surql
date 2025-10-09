-- =========================================================
-- Chaosmith SurrealDB Schema (L1) — SurrealDB 2.2.2
-- Specs: PCS/1.3-native + UDCS/1.0 + PCS/INST/1.0
-- =========================================================

-- ==== DENS (physical locations) ====
DEFINE TABLE den SCHEMAFULL;
DEFINE FIELD code      ON den TYPE string ASSERT $value != "";
DEFINE FIELD name      ON den TYPE string;
DEFINE FIELD address   ON den TYPE string;
DEFINE FIELD geo       ON den TYPE object;                 -- {lat,lon,alt?}
DEFINE FIELD desc      ON den TYPE string;
DEFINE FIELD tags      ON den TYPE array<string>;
DEFINE FIELD inventory ON den TYPE object;                 -- {nodes,iot,agents,tasks}
DEFINE INDEX uniq_den ON TABLE den COLUMNS code UNIQUE;

-- ==== NODES (machines) ====
DEFINE TABLE node SCHEMAFULL;
DEFINE FIELD name   ON node TYPE string ASSERT $value != "";
DEFINE FIELD kind   ON node TYPE string;                   -- "pc","headless","arm","vm"
DEFINE FIELD os     ON node TYPE string;
DEFINE FIELD cpu    ON node TYPE string;
DEFINE FIELD ram_gb ON node TYPE int;
DEFINE FIELD labels ON node TYPE array<string>;
DEFINE INDEX uniq_node ON TABLE node COLUMNS name UNIQUE;

-- ==== NODE FILES (docs/config on a node) ====
DEFINE TABLE node_file SCHEMAFULL;
DEFINE FIELD node   ON node_file TYPE record<node>;
DEFINE FIELD path   ON node_file TYPE string;              -- absolute path
DEFINE FIELD size   ON node_file TYPE int;
DEFINE FIELD mtime  ON node_file TYPE datetime;
DEFINE FIELD sha    ON node_file TYPE string;              -- BLAKE3/SHA-256
DEFINE INDEX uniq_node_path ON TABLE node_file COLUMNS node, path UNIQUE;

-- ==== API REFERENCES (URL or local node_file) ====
DEFINE TABLE api_ref SCHEMAFULL;
DEFINE FIELD ref_type   ON api_ref TYPE string;            -- "url" | "file"
DEFINE FIELD url        ON api_ref TYPE string;            -- iff ref_type="url"
DEFINE FIELD file       ON api_ref TYPE option<record<node_file>>; -- iff ref_type="file"
DEFINE FIELD title      ON api_ref TYPE string;
DEFINE FIELD version    ON api_ref TYPE string;
DEFINE FIELD updated_at ON api_ref TYPE datetime;
DEFINE FIELD tags       ON api_ref TYPE array<string>..;

-- ==== IoT DEVICES ====
DEFINE TABLE iot_device SCHEMAFULL;
DEFINE FIELD den    ON iot_device TYPE record<den>;
DEFINE FIELD hwid   ON iot_device TYPE string;             -- MAC/serial
DEFINE FIELD model  ON iot_device TYPE string;             -- "shelly-plug-s","esp32-s3"
DEFINE FIELD role   ON iot_device TYPE string;             -- "sensor","actuator","cam"
DEFINE FIELD status ON iot_device TYPE string;             -- "up","down","unknown"
DEFINE FIELD api    ON iot_device TYPE option<record<api_ref>>;
DEFINE INDEX uniq_iot ON TABLE iot_device COLUMNS den, hwid UNIQUE;

-- ==== AGENTS (daemons/routines) ====
DEFINE TABLE agent SCHEMAFULL;
DEFINE FIELD ident    ON agent TYPE string;                -- "orchd","indexer-go"
DEFINE FIELD version  ON agent TYPE string;
DEFINE FIELD node     ON agent TYPE record<node>;
DEFINE FIELD den      ON agent TYPE record<den>;
DEFINE FIELD status   ON agent TYPE string;                -- "idle","running","dead"
DEFINE INDEX uniq_agent ON TABLE agent COLUMNS ident, node UNIQUE;

-- ==== TASKS (summary only; detailed logs live elsewhere) ====
DEFINE TABLE task SCHEMAFULL;
DEFINE FIELD run_id      ON task TYPE string;              -- RUN-YYYYMMDD-xxxx
DEFINE FIELD agent       ON task TYPE record<agent>;
DEFINE FIELD ws          ON task TYPE option<record<workspace>>;
DEFINE FIELD state       ON task TYPE string;              -- "queued","running","done","error"
DEFINE FIELD started_at  ON task TYPE datetime;
DEFINE FIELD finished_at ON task TYPE datetime;

-- ==== WORKSPACES (live on nodes; also linked to dens for inventory) ====
DEFINE TABLE workspace SCHEMAFULL;
DEFINE FIELD path        ON workspace TYPE string ASSERT $value != "";
DEFINE FIELD node        ON workspace TYPE record<node> ASSERT $value != NONE;
DEFINE FIELD vcs         ON workspace TYPE string;          -- "git", etc.
DEFINE FIELD rev         ON workspace TYPE string;          -- commit/tag
DEFINE FIELD content_sha ON workspace TYPE string;          -- hash of file list
DEFINE INDEX uniq_ws ON TABLE workspace COLUMNS node, path UNIQUE;

-- ==== DIRECTORIES ====
DEFINE TABLE directory SCHEMAFULL;
DEFINE FIELD ws      ON directory TYPE record<workspace> ASSERT $value != NONE;
DEFINE FIELD relpath ON directory TYPE string;              -- "" for root
DEFINE FIELD sha     ON directory TYPE string;
DEFINE INDEX uniq_dir ON TABLE directory COLUMNS ws, relpath UNIQUE;

-- ==== FILES ====
DEFINE TABLE file SCHEMAFULL;
DEFINE FIELD ws      ON file TYPE record<workspace> ASSERT $value != NONE;
DEFINE FIELD relpath ON file TYPE string;
DEFINE FIELD lang    ON file TYPE string;                   -- from extension map
DEFINE FIELD size    ON file TYPE int;
DEFINE FIELD mtime   ON file TYPE datetime;
DEFINE FIELD sha     ON file TYPE string;
DEFINE INDEX uniq_file ON TABLE file COLUMNS ws, relpath UNIQUE;

-- ==== SYMBOLS (definitions only at L1) ====
DEFINE TABLE symbol SCHEMAFULL;
DEFINE FIELD ws      ON symbol TYPE record<workspace> ASSERT $value != NONE;
DEFINE FIELD file    ON symbol TYPE record<file> ASSERT $value != NONE;
DEFINE FIELD name    ON symbol TYPE string;
DEFINE FIELD fqname  ON symbol TYPE string;                 -- workspace-scoped
DEFINE FIELD kind    ON symbol TYPE string;                 -- "func","type","class","var","endpoint","table"
DEFINE FIELD lang    ON symbol TYPE string;
DEFINE FIELD range   ON symbol TYPE object;                 -- {start:{l,c}, end:{l,c}}
DEFINE INDEX uniq_sym ON TABLE symbol COLUMNS ws, fqname UNIQUE;

-- ==== VECTOR MODELS (provenance) ====
DEFINE TABLE vector_model SCHEMAFULL;
DEFINE FIELD id_slug    ON vector_model TYPE string ASSERT $value != "";
DEFINE FIELD family     ON vector_model TYPE string;        -- "mxbai","bge","e5","codebert"
DEFINE FIELD version    ON vector_model TYPE string;        -- "large","2507", etc.
DEFINE FIELD native_dim ON vector_model TYPE int;           -- raw output dim
DEFINE FIELD notes      ON vector_model TYPE string;
DEFINE INDEX uniq_vm ON TABLE vector_model COLUMNS id_slug UNIQUE;

-- ==== VECTOR CHUNKS (symbol/file spans with embeddings) ====
DEFINE TABLE vector_chunk SCHEMAFULL;
DEFINE FIELD ws            ON vector_chunk TYPE record<workspace>;
DEFINE FIELD file          ON vector_chunk TYPE record<file>;
DEFINE FIELD symbol        ON vector_chunk TYPE option<record<symbol>>;
DEFINE FIELD granularity   ON vector_chunk TYPE string;           -- "symbol" | "file" | "span" | "file_chunk"
DEFINE FIELD chunk_index   ON vector_chunk TYPE option<int>;
DEFINE FIELD start         ON vector_chunk TYPE option<int>;
DEFINE FIELD end           ON vector_chunk TYPE option<int>;
DEFINE FIELD token_count   ON vector_chunk TYPE option<int>;
DEFINE FIELD content_sha   ON vector_chunk TYPE string;           -- hash of the exact text span
DEFINE FIELD model         ON vector_chunk TYPE record<vector_model>;
DEFINE FIELD model_sha     ON vector_chunk TYPE string;           -- hash of model files/config
DEFINE FIELD native_dim    ON vector_chunk TYPE int;
DEFINE FIELD effective_dim ON vector_chunk TYPE int;              -- after PCA/etc
DEFINE FIELD transform_id  ON vector_chunk TYPE string;           -- "none" | "pca-256@<hash>"
DEFINE FIELD vector        ON vector_chunk TYPE array<float>;            -- array<float>
DEFINE FIELD ts            ON vector_chunk TYPE datetime;
DEFINE INDEX uniq_vc ON TABLE vector_chunk
  COLUMNS ws, file, symbol, granularity, start, end, model UNIQUE;

-- ==== WORKSPACE VECTORS (centroids/fingerprints) ====
DEFINE TABLE workspace_vector SCHEMAFULL;
DEFINE FIELD ws      ON workspace_vector TYPE record<workspace>;
DEFINE FIELD kind    ON workspace_vector TYPE string;             -- "centroid@symbol","centroid@file"
DEFINE FIELD model   ON workspace_vector TYPE record<vector_model>;
DEFINE FIELD vector  ON workspace_vector TYPE array<float>;
DEFINE FIELD sample  ON workspace_vector TYPE int;                -- contributing chunks
DEFINE FIELD ts      ON workspace_vector TYPE datetime;
DEFINE INDEX uniq_wsv ON TABLE workspace_vector COLUMNS ws, kind, model UNIQUE;

-- =========================================================
-- Relations (split per pair; no comma lists in 2.2.2)
-- =========================================================

-- Den inventories
DEFINE TABLE den_has_node       TYPE RELATION FROM den TO node;
DEFINE TABLE den_has_iot        TYPE RELATION FROM den TO iot_device;
DEFINE TABLE den_has_agent      TYPE RELATION FROM den TO agent;
DEFINE TABLE den_has_task       TYPE RELATION FROM den TO task;
DEFINE TABLE den_has_workspace  TYPE RELATION FROM den TO workspace;

-- Node files live on nodes
DEFINE TABLE node_file_stored_on TYPE RELATION FROM node_file TO node;

-- IoT devices have API refs
DEFINE TABLE iot_has_api TYPE RELATION FROM iot_device TO api_ref;

-- Workspaces live on nodes
DEFINE TABLE on_node TYPE RELATION FROM workspace TO node;

-- Containment
DEFINE TABLE ws_contains_dir    TYPE RELATION FROM workspace TO directory;
DEFINE TABLE dir_contains_dir   TYPE RELATION FROM directory TO directory;
DEFINE TABLE dir_contains_file  TYPE RELATION FROM directory TO file;
DEFINE TABLE file_contains_sym  TYPE RELATION FROM file      TO symbol;

-- Definitions
DEFINE TABLE defines TYPE RELATION FROM file TO symbol;

-- Vectors
DEFINE TABLE symbol_has_vector    TYPE RELATION FROM symbol    TO vector_chunk;
DEFINE TABLE file_has_vector      TYPE RELATION FROM file      TO vector_chunk;
DEFINE TABLE workspace_has_vector TYPE RELATION FROM workspace TO workspace_vector;

-- Vector indexes
-- KNN for chunk-level search
DEFINE INDEX idx_vector_chunk_hnsw
  ON TABLE vector_chunk
  FIELDS vector
  HNSW DIMENSION 768 DIST COSINE;

-- For centroid/fingerprint search
DEFINE INDEX idx_workspace_vector_hnsw
  ON TABLE workspace_vector
  FIELDS vector
  HNSW DIMENSION 768 DIST COSINE;

-- =========================================================
-- End of schema
-- =========================================================

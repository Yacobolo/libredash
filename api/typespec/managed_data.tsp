using Http;
using TypeSpec.OpenAPI;

@route("/projects/{project}/data-connections/{connection}")
namespace LibreDashAPI.ManagedData;

@minLength(1)
@maxLength(128)
@pattern("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
scalar ManagedDataProjectId extends string;

@minLength(1)
@maxLength(128)
@pattern("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
scalar ManagedDataConnectionId extends string;

@minLength(1)
@maxLength(128)
@pattern("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
scalar ManagedDataEnvironmentId extends string;

@minLength(1)
@maxLength(128)
@pattern("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
scalar ManagedDataWorkspaceId extends string;

@minLength(1)
@maxLength(160)
scalar ManagedDataServingStateId extends string;

@minLength(1)
@maxLength(160)
scalar ManagedDataUploadSessionId extends string;

@minLength(1)
@maxLength(160)
scalar ManagedDataTusUploadId extends string;

@minLength(1)
@maxLength(160)
scalar ManagedDataMultipartUploadId extends string;

@minLength(1)
@maxLength(255)
scalar ManagedDataIdempotencyKey extends string;

@minLength(1)
@maxLength(2048)
scalar ManagedDataPageToken extends string;

@minLength(1)
@maxLength(1024)
@pattern("^(?!/)(?!.*(?:^|/)\\.\\.(?:/|$))(?!.*\\\\).+$")
scalar ManagedDataLogicalPath extends string;

@pattern("^[0-9a-f]{64}$")
@minLength(64)
@maxLength(64)
@format("sha256")
scalar ManagedDataFileSha256 extends string;

@pattern("^sha256:[0-9a-f]{64}$")
@minLength(71)
@maxLength(71)
@format("sha256-revision")
scalar ManagedDataRevisionId extends string;

@minValue(0)
scalar ManagedDataByteCount extends int64;

@minValue(1)
scalar ManagedDataPositiveByteCount extends int64;

@minValue(1)
@maxValue(10000)
scalar ManagedDataFileCount extends int32;

@minValue(1)
@maxValue(200)
scalar ManagedDataPageLimit extends int32;

@minValue(1)
@maxValue(10000)
scalar ManagedDataS3PartNumber extends int32;

@minLength(1)
@maxLength(8192)
scalar ManagedDataEndpoint extends string;

@minLength(1)
@maxLength(256)
scalar ManagedDataHeaderName extends string;

@minLength(1)
@maxLength(8192)
scalar ManagedDataHeaderValue extends string;

@minLength(1)
@maxLength(2048)
scalar ManagedDataErrorMessage extends string;

enum ManagedDataRevisionStatus {
  available;
}

enum ManagedDataUploadSessionStatus {
  open;
  finalizing;
  completed;
  aborted;
  failed;
  expired;
}

enum ManagedDataFileUploadStatus {
  pending;
  uploading;
  uploaded;
  verified;
  skipped;
  failed;
}

enum ManagedDataUploadProtocol {
  tus;
  s3Multipart: "s3_multipart";
  alreadyPresent: "already_present";
}

enum ManagedDataS3MultipartStatus {
  open;
  completed;
  aborted;
}

model ManagedDataFileMetadata {
  @pattern("^(?!/)(?!.*(?:^|/)\\.\\.(?:/|$))(?!.*\\\\).+$")
  path: ManagedDataLogicalPath;
  size: ManagedDataByteCount;
  @doc("A lowercase 64-character hexadecimal SHA-256 digest.")
  @pattern("^[0-9a-f]{64}$")
  @minLength(64)
  @maxLength(64)
  @format("sha256")
  sha256: ManagedDataFileSha256;
}

model ManagedDataManifest {
  @minItems(1)
  @maxItems(10000)
  files: ManagedDataFileMetadata[];
}

model ManagedDataRevisionSummaryResponse {
  @doc("The immutable revision identity, formatted as sha256 followed by a colon and 64 lowercase hexadecimal characters.")
  @pattern("^sha256:[0-9a-f]{64}$")
  @minLength(71)
  @maxLength(71)
  @format("sha256-revision")
  id: ManagedDataRevisionId;
  status: ManagedDataRevisionStatus;
  fileCount: ManagedDataFileCount;
  size: ManagedDataByteCount;
  createdAt: utcDateTime;
  uploadSessionId: ManagedDataUploadSessionId;
}

model ManagedDataRevisionResponse {
  @doc("The immutable revision identity, formatted as sha256 followed by a colon and 64 lowercase hexadecimal characters.")
  @pattern("^sha256:[0-9a-f]{64}$")
  @minLength(71)
  @maxLength(71)
  @format("sha256-revision")
  id: ManagedDataRevisionId;
  status: ManagedDataRevisionStatus;
  manifest: ManagedDataManifest;
  fileCount: ManagedDataFileCount;
  size: ManagedDataByteCount;
  createdAt: utcDateTime;
  uploadSessionId: ManagedDataUploadSessionId;
}

model ManagedDataRevisionListResponse {
  @maxItems(200)
  items: ManagedDataRevisionSummaryResponse[];
  page: PageInfo;
}

model ManagedDataEnvironmentRevisionResponse {
  environment: ManagedDataEnvironmentId;
  revision?: ManagedDataRevisionSummaryResponse;
  deploymentId?: ProjectDeploymentId;
  activatedAt?: utcDateTime;
}

model ManagedDataHTTPHeader {
  name: ManagedDataHeaderName;
  value: ManagedDataHeaderValue;
}

model ManagedDataTusUploadNegotiation {
  endpoint: ManagedDataEndpoint;
  uploadId: ManagedDataTusUploadId;
  offset: ManagedDataByteCount;
  expiresAt: utcDateTime;
}

model ManagedDataS3MultipartNegotiation {
  createEndpoint: ManagedDataEndpoint;
  minimumPartSize: ManagedDataPositiveByteCount;
  maximumPartSize: ManagedDataPositiveByteCount;
  maximumParts: ManagedDataS3PartNumber;
}

@doc("Backend-neutral upload instructions. Exactly one protocol-specific object is present for tus or S3 multipart; already-present files have neither.")
model ManagedDataUploadNegotiation {
  protocol: ManagedDataUploadProtocol;
  tus?: ManagedDataTusUploadNegotiation;
  s3Multipart?: ManagedDataS3MultipartNegotiation;
}

model ManagedDataFileUploadResponse {
  file: ManagedDataFileMetadata;
  status: ManagedDataFileUploadStatus;
  negotiation: ManagedDataUploadNegotiation;
  error?: ManagedDataErrorMessage;
}

model ManagedDataUploadSessionCreateRequest {
  manifest: ManagedDataManifest;
}

model ManagedDataUploadSessionResponse {
  id: ManagedDataUploadSessionId;
  project: ManagedDataProjectId;
  connection: ManagedDataConnectionId;
  @pattern("^sha256:[0-9a-f]{64}$")
  revisionId: ManagedDataRevisionId;
  status: ManagedDataUploadSessionStatus;
  manifest: ManagedDataManifest;
  @minItems(1)
  @maxItems(10000)
  files: ManagedDataFileUploadResponse[];
  createdAt: utcDateTime;
  expiresAt: utcDateTime;
  completedAt?: utcDateTime;
  error?: ManagedDataErrorMessage;
}

model ManagedDataS3MultipartCreateRequest {
  path: ManagedDataLogicalPath;
}

model ManagedDataS3MultipartUploadResponse {
  id: ManagedDataMultipartUploadId;
  uploadSessionId: ManagedDataUploadSessionId;
  file: ManagedDataFileMetadata;
  status: ManagedDataS3MultipartStatus;
  existing: boolean;
  createdAt: utcDateTime;
  expiresAt?: utcDateTime;
}

model ManagedDataS3MultipartSignPartRequest {
  size: ManagedDataPositiveByteCount;
  @pattern("^[0-9a-f]{64}$")
  sha256?: ManagedDataFileSha256;
}

model ManagedDataS3MultipartSignedPartResponse {
  partNumber: ManagedDataS3PartNumber;
  url: ManagedDataEndpoint;
  @maxItems(32)
  headers: ManagedDataHTTPHeader[];
  expiresAt: utcDateTime;
}

model ManagedDataS3CompletedPart {
  partNumber: ManagedDataS3PartNumber;
  @minLength(1)
  @maxLength(1024)
  etag: string;
  @pattern("^[0-9a-f]{64}$")
  sha256?: ManagedDataFileSha256;
}

model ManagedDataS3MultipartCompleteRequest {
  @minItems(1)
  @maxItems(10000)
  parts: ManagedDataS3CompletedPart[];
}

@tag("Managed Data")
@summary("Get the current managed-data revision for an environment")
@route("/environments/{environment}/revision")
@get
@operationId("getManagedDataEnvironmentRevision")
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_DATA" })
op getManagedDataEnvironmentRevision(
  @path project: ManagedDataProjectId,
  @path connection: ManagedDataConnectionId,
  @path environment: ManagedDataEnvironmentId,
): OkJson<ManagedDataEnvironmentRevisionResponse> | CommonErrors;

@tag("Managed Data")
@summary("List immutable managed-data revisions")
@route("/revisions")
@get
@operationId("listManagedDataRevisions")
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_DATA" })
op listManagedDataRevisions(
  @path project: ManagedDataProjectId,
  @path connection: ManagedDataConnectionId,
  @query limit?: ManagedDataPageLimit,
  @query pageToken?: ManagedDataPageToken,
): OkJson<ManagedDataRevisionListResponse> | CommonErrors;

@tag("Managed Data")
@summary("Get an immutable managed-data revision")
@route("/revisions/{revision}")
@get
@operationId("getManagedDataRevision")
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_DATA" })
op getManagedDataRevision(
  @path project: ManagedDataProjectId,
  @path connection: ManagedDataConnectionId,
  @path revision: ManagedDataRevisionId,
): OkJson<ManagedDataRevisionResponse> | CommonErrors;

@tag("Managed Data")
@summary("Create a managed-data upload session from a canonical manifest")
@route("/upload-sessions")
@post
@operationId("createManagedDataUploadSession")
@apigen.authz(#{ mode: "privilege", privilege: "INGEST_DATA" })
op createManagedDataUploadSession(
  @path project: ManagedDataProjectId,
  @path connection: ManagedDataConnectionId,
  @header("Idempotency-Key") idempotencyKey: ManagedDataIdempotencyKey,
  @body body: ManagedDataUploadSessionCreateRequest,
): CreatedJson<ManagedDataUploadSessionResponse> | CommonErrors;

@tag("Managed Data")
@summary("Get a managed-data upload session")
@route("/upload-sessions/{uploadSession}")
@get
@operationId("getManagedDataUploadSession")
@apigen.authz(#{ mode: "privilege", privilege: "INGEST_DATA" })
op getManagedDataUploadSession(
  @path project: ManagedDataProjectId,
  @path connection: ManagedDataConnectionId,
  @path uploadSession: ManagedDataUploadSessionId,
): OkJson<ManagedDataUploadSessionResponse> | CommonErrors;

@tag("Managed Data")
@summary("Abort a managed-data upload session")
@route("/upload-sessions/{uploadSession}/abort")
@post
@operationId("abortManagedDataUploadSession")
@apigen.authz(#{ mode: "privilege", privilege: "INGEST_DATA" })
op abortManagedDataUploadSession(
  @path project: ManagedDataProjectId,
  @path connection: ManagedDataConnectionId,
  @path uploadSession: ManagedDataUploadSessionId,
  @header("Idempotency-Key") idempotencyKey: ManagedDataIdempotencyKey,
): OkJson<ManagedDataUploadSessionResponse> | CommonErrors;

@tag("Managed Data")
@summary("Finalize a managed-data upload session")
@route("/upload-sessions/{uploadSession}/finalize")
@post
@operationId("finalizeManagedDataUploadSession")
@apigen.authz(#{ mode: "privilege", privilege: "INGEST_DATA" })
op finalizeManagedDataUploadSession(
  @path project: ManagedDataProjectId,
  @path connection: ManagedDataConnectionId,
  @path uploadSession: ManagedDataUploadSessionId,
  @header("Idempotency-Key") idempotencyKey: ManagedDataIdempotencyKey,
): AcceptedJson<ManagedDataUploadSessionResponse> | CommonErrors;

@tag("Managed Data")
@summary("Create an S3 multipart upload for a session file")
@route("/upload-sessions/{uploadSession}/s3-multipart-uploads")
@post
@operationId("createManagedDataS3MultipartUpload")
@apigen.authz(#{ mode: "privilege", privilege: "INGEST_DATA" })
op createManagedDataS3MultipartUpload(
  @path project: ManagedDataProjectId,
  @path connection: ManagedDataConnectionId,
  @path uploadSession: ManagedDataUploadSessionId,
  @header("Idempotency-Key") idempotencyKey: ManagedDataIdempotencyKey,
  @body body: ManagedDataS3MultipartCreateRequest,
): CreatedJson<ManagedDataS3MultipartUploadResponse> | CommonErrors;

@tag("Managed Data")
@summary("Sign one S3 multipart upload part")
@route("/upload-sessions/{uploadSession}/s3-multipart-uploads/{multipartUpload}/parts/{partNumber}/sign")
@post
@operationId("signManagedDataS3MultipartPart")
@apigen.authz(#{ mode: "privilege", privilege: "INGEST_DATA" })
op signManagedDataS3MultipartPart(
  @path project: ManagedDataProjectId,
  @path connection: ManagedDataConnectionId,
  @path uploadSession: ManagedDataUploadSessionId,
  @path multipartUpload: ManagedDataMultipartUploadId,
  @path partNumber: ManagedDataS3PartNumber,
  @body body: ManagedDataS3MultipartSignPartRequest,
): OkJson<ManagedDataS3MultipartSignedPartResponse> | CommonErrors;

@tag("Managed Data")
@summary("Complete an S3 multipart upload")
@route("/upload-sessions/{uploadSession}/s3-multipart-uploads/{multipartUpload}/complete")
@post
@operationId("completeManagedDataS3MultipartUpload")
@apigen.authz(#{ mode: "privilege", privilege: "INGEST_DATA" })
op completeManagedDataS3MultipartUpload(
  @path project: ManagedDataProjectId,
  @path connection: ManagedDataConnectionId,
  @path uploadSession: ManagedDataUploadSessionId,
  @path multipartUpload: ManagedDataMultipartUploadId,
  @header("Idempotency-Key") idempotencyKey: ManagedDataIdempotencyKey,
  @body body: ManagedDataS3MultipartCompleteRequest,
): OkJson<ManagedDataS3MultipartUploadResponse> | CommonErrors;

@tag("Managed Data")
@summary("Abort an S3 multipart upload")
@route("/upload-sessions/{uploadSession}/s3-multipart-uploads/{multipartUpload}/abort")
@post
@operationId("abortManagedDataS3MultipartUpload")
@apigen.authz(#{ mode: "privilege", privilege: "INGEST_DATA" })
op abortManagedDataS3MultipartUpload(
  @path project: ManagedDataProjectId,
  @path connection: ManagedDataConnectionId,
  @path uploadSession: ManagedDataUploadSessionId,
  @path multipartUpload: ManagedDataMultipartUploadId,
  @header("Idempotency-Key") idempotencyKey: ManagedDataIdempotencyKey,
): OkJson<ManagedDataS3MultipartUploadResponse> | CommonErrors;

using Http;
using TypeSpec.OpenAPI;

namespace LeapViewAPI.ManagedData;

@minLength(1)
@maxLength(160)
scalar ManagedDataMultipartUploadId extends string;

@minValue(1)
@maxValue(10000)
scalar ManagedDataS3PartNumber extends int32;

enum ManagedDataS3MultipartStatus {
  open;
  completed;
  aborted;
}

@doc("Negotiated S3 multipart transport. Commands below manage transport only; upload-session finalization remains a separate lifecycle operation.")
model ManagedDataS3MultipartNegotiation {
  createEndpoint: ManagedDataEndpoint;
  minimumPartSize: ManagedDataPositiveByteCount;
  maximumPartSize: ManagedDataPositiveByteCount;
  maximumParts: ManagedDataS3PartNumber;
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
  @header("Idempotency-Key") idempotencyKey: ManagedDataIdempotencyKey,
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

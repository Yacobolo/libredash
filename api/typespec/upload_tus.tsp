using Http;
using TypeSpec.OpenAPI;

namespace LibreDashAPI.ManagedData;

@minLength(1)
@maxLength(160)
scalar ManagedDataTusUploadId extends string;

@doc("Negotiated TUS transport. The endpoint implements the TUS protocol and remains separate from upload-session lifecycle commands.")
model ManagedDataTusUploadNegotiation {
  endpoint: ManagedDataEndpoint;
  uploadId: ManagedDataTusUploadId;
  offset: ManagedDataByteCount;
  expiresAt: utcDateTime;
}

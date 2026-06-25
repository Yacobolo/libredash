using Http;
using TypeSpec.OpenAPI;

namespace LibreDashAPI;

model Error {
  code: int32;
  message: string;
  details?: Record<string>;
  requestId?: string;
}

model PageInfo {
  nextCursor?: string;
}

model StatusResponse {
  status: string;
}

model BadRequest {
  @statusCode statusCode: 400;
  @body body: Error;
}

model Unauthorized {
  @statusCode statusCode: 401;
  @body body: Error;
}

model Forbidden {
  @statusCode statusCode: 403;
  @body body: Error;
}

model NotFound {
  @statusCode statusCode: 404;
  @body body: Error;
}

model Conflict {
  @statusCode statusCode: 409;
  @body body: Error;
}

model RateLimited {
  @statusCode statusCode: 429;
  @body body: Error;
}

model InternalServerError {
  @statusCode statusCode: 500;
  @body body: Error;
}

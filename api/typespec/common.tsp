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

model OkJson<T> {
  ...OkResponse;
  ...Body<T>;
}

model CreatedJson<T> {
  ...CreatedResponse;
  ...Body<T>;
}

model AcceptedJson<T> {
  ...Response<202>;
  ...Body<T>;
}

model ErrorResponse<Status> {
  ...Response<Status>;
  ...Body<Error>;
}

model BadRequest {
  ...BadRequestResponse;
  ...Body<Error>;
}

model Unauthorized {
  ...UnauthorizedResponse;
  ...Body<Error>;
}

model Forbidden {
  ...ForbiddenResponse;
  ...Body<Error>;
}

model NotFound {
  ...NotFoundResponse;
  ...Body<Error>;
}

model Conflict {
  ...ConflictResponse;
  ...Body<Error>;
}

model RateLimited {
  ...ErrorResponse<429>;
}

model InternalServerError {
  ...ErrorResponse<500>;
}

alias CommonErrors = BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

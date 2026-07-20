using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LeapViewAPI;

model InstanceResponse {
  environment: string;
}

@tag("Instance")
@summary("Get this LeapView instance identity")
@route("/instance")
@get
@apigen.authz(#{ mode: "authenticated" })
op getInstance(): OkJson<InstanceResponse> | CommonErrors;

using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LibreDashAPI;

model InstanceResponse {
  environment: string;
}

@tag("Instance")
@summary("Get this LibreDash instance identity")
@route("/instance")
@get
@apigen.authz(#{ mode: "authenticated" })
op getInstance(): OkJson<InstanceResponse> | CommonErrors;

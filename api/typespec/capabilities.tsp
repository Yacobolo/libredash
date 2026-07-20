using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LeapViewAPI;

enum AuthenticationMode {
  bearer,
}

enum QueryFormat {
  json: "application/json",
  arrow: "application/vnd.apache.arrow.stream",
}

enum UploadProtocol {
  tus,
  s3Multipart: "s3_multipart",
}

enum VisualShape {
  categoryValue: "category_value",
  categorySeriesValue: "category_series_value",
  categoryMultiMeasure: "category_multi_measure",
  categoryDelta: "category_delta",
  binnedMeasure: "binned_measure",
  hierarchy,
  singleValue: "single_value",
  matrix,
  graph,
  geo,
  ohlc,
  distribution,
}

model CapabilitiesResponse {
  apiVersion: string;
  buildVersion: string;
  environment: string;
  authentication: AuthenticationMode[];
  queryFormats: QueryFormat[];
  uploadProtocols: UploadProtocol[];
  visualShapes: VisualShape[];
}

@tag("System")
@summary("Describe server capabilities")
@route("/capabilities")
@get
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
op getCapabilities(): OkJson<CapabilitiesResponse> | CommonErrors;

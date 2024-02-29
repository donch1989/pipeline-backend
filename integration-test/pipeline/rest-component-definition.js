import http from "k6/http";
import { check, group } from "k6";

import { pipelinePublicHost } from "./const.js";

export function CheckList() {
  group("Component API: List component definitions", () => {
    // Default pagination.
    var defaultPageSize = 10;
    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions`, null, null), {
      "GET /v1beta/component-definitions response status is 200": (r) => r.status === 200,
      "GET /v1beta/component-definitions response has component_definitions array": (r) => Array.isArray(r.json().component_definitions),
      "GET /v1beta/component-definitions response total_size > 0": (r) => r.json().total_size > 0,
      "GET /v1beta/component-definitions response page 0": (r) => r.json().page === 0,
      [`GET /v1beta/component-definitions response default page size ${defaultPageSize}`]: (r) => r.json().component_definitions.length === defaultPageSize,
      [`GET /v1beta/component-definitions response page size in response ${defaultPageSize}`]: (r) => r.json().page_size === defaultPageSize
    });

    var limitedRecords = http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions`, null, null)

    // Page size 0.
    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions?page_size=0`, null, null), {
      "GET /v1beta/component-definitions?page_size=0 response status is 200": (r) => r.status === 200,
      "GET /v1beta/component-definitions?page_size=0 response default page size": (r) => r.json().component_definitions.length === limitedRecords.json().component_definitions.length,
    });

    // Negative page size.
    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions?page_size=-1`, null, null), {
      "GET /v1beta/component-definitions?page_size=-1 response status is 200": (r) => r.status === 200,
      "GET /v1beta/component-definitions?page_size=-1 response default page size": (r) => r.json().component_definitions.length === limitedRecords.json().component_definitions.length,
    });

    // Valid, non-default page size.
    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions?page_size=1`, null, null), {
      "GET /v1beta/component-definitions?page_size=1 response status is 200": (r) => r.status === 200,
      "GET /v1beta/component-definitions?page_size=1 response component_definitions size 1": (r) => r.json().component_definitions.length === 1,
    });

    // Page size over total records.
    var bigPage = limitedRecords.json().total_size + 10
    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions?page_size=${bigPage}`, null, null), {
      [`GET /v1beta/component-definitions?page_size=${bigPage} response status 200`]: (r) => r.status === 200,
      [`GET /v1beta/component-definitions?page_size=${bigPage} response component_definitions size ${limitedRecords.json().total_size }`]: (r) => r.json().component_definitions.length === limitedRecords.json().total_size,
    });

    // Access non-first page.
    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions?page_size=3&page=2`, null, null), {
      "GET /v1beta/component-definitions?page_size=3&page=2 response status is 200": (r) => r.status === 200,
      "GET /v1beta/component-definitions?page_size=3&page=2 response component_definitions size 3": (r) => r.json().component_definitions.length === 3,
      "GET /v1beta/component-definitions?page_size=3&page=2 response page 0": (r) => r.json().page === 2,
      "GET /v1beta/component-definitions?page_size=3&page=2 receives a different page": (r) => r.json().component_definitions[0].connector_definition.id != limitedRecords.json().component_definitions[0].connector_definition.id,
    });

    // Negative page index yields page 0.
    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions?page_size=3&page=-2`, null, null), {
      "GET /v1beta/component-definitions?page_size=3&page=-2 response status is 200": (r) => r.status === 200,
      "GET /v1beta/component-definitions?page_size=3&page=-2 response component_definitions size 3": (r) => r.json().component_definitions.length === 3,
      "GET /v1beta/component-definitions?page_size=3&page=-2 response page 0": (r) => r.json().page === 0,
    });

    // Page index beyond last page.
    var bigPage = limitedRecords.json().total_size + 10
    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions?page_size=${bigPage}&page=2`, null, null), {
      [`GET /v1beta/component-definitions?page_size=${bigPage}&page=2 response status 200`]: (r) => r.status === 200,
      [`GET /v1beta/component-definitions?page_size=${bigPage}&page=2 response component_definitions size 0`]: (r) => r.json().component_definitions.length === 0,
    });

    // Default view is BASIC, i.e. no spec property.
    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions?page_size=1`, null, null), {
      "GET /v1beta/component-definitions?page_size=1 response status 200": (r) => r.status === 200,
      "GET /v1beta/component-definitions?page_size=1 response component_definitions[0].connector_definition.spec is null": (r) => r.json().component_definitions[0].connector_definition.spec === null,
    });

    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions?page_size=1&view=VIEW_BASIC`, null, null), {
      "GET /v1beta/component-definitions?page_size=1&view=VIEW_BASIC response status 200": (r) => r.status === 200,
      "GET /v1beta/component-definitions?page_size=1&view=VIEW_BASIC response component_definitions[0].connector_definition.spec is null": (r) => r.json().component_definitions[0].connector_definition.spec === null,
    });

    // FULL view.
    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions?page_size=1&view=VIEW_FULL`, null, null), {
      "GET /v1beta/component-definitions?page_size=1&view=VIEW_FULL response status 200": (r) => r.status === 200,
      "GET /v1beta/component-definitions?page_size=1&view=VIEW_FULL response component_definitions[0].connector_definition.spec is not null": (r) => r.json().component_definitions[0].connector_definition.spec !== null,
    });


    // Fetcha page with operator definitions.
    // TODO when there are more connector definitions than the max page size
    // (100), accessing the 2nd page won't work. We'll need to use a smaller
    // page size and compute the page where operator definitions start.
    var connectorRecords = http.request("GET", `${pipelinePublicHost}/v1beta/connector-definitions?page_size=1`, null, null)
    var connectorSize = connectorRecords.json().total_size
    check(http.request("GET", `${pipelinePublicHost}/v1beta/component-definitions?page_size=${connectorSize}&page=1`, null, null), {
      [`GET /v1beta/component-definitions?page_size=${connectorSize}&page=1 response status 200`]: (r) => r.status === 200,
      [`GET /v1beta/component-definitions?page_size=${connectorSize}&page=1 response contains operator definition type`]: (r) => r.json().component_definitions[0].type === "COMPONENT_TYPE_OPERATOR",
      [`GET /v1beta/component-definitions?page_size=${connectorSize}&page=1 response contains operator definitions`]: (r) => r.json().component_definitions[0].operator_definition.id != "",
    });
  });
}

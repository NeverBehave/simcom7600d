import { defineConfig } from "@hey-api/openapi-ts";

export default defineConfig({
  input: "../../internal/api/openapi.json",
  output: {
    path: "src",
    postProcess: [],
  },
  client: "@hey-api/client-fetch",
  types: {
    enums: "typescript",
  },
  services: {
    asClass: false,
  },
});

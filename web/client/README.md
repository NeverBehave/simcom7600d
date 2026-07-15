# @sim7600d/client

Typed TypeScript SDK for the SIM7600D control API. Generated from
`internal/api/openapi.json` by `@hey-api/openapi-ts`.

## Regenerate

From the repo root:

```
make sdk
```

Or from `web/client/`:

```
npm install
npm run gen
npm run typecheck
```

## Usage

```ts
import { Sim7600Client } from "@sim7600d/client";

const sim = new Sim7600Client({
  baseUrl: "http://127.0.0.1:8080",
  token: process.env.SIM7600D_TOKEN!,
});

const { data, error } = await sim.sms.send({
  body: { to: "+15551234567", body: "hello" },
});
if (error) throw new Error(error.error.message);
console.log(data.id, data.state);
```

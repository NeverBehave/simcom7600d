import { test } from "node:test";
import assert from "node:assert/strict";

import { Sim7600Client } from "../index.ts";

const url = process.env.SIM7600D_URL;
const token = process.env.SIM7600D_TOKEN;
if (!url || !token) {
  throw new Error("SIM7600D_URL and SIM7600D_TOKEN must be set");
}

const sim = new Sim7600Client({ baseUrl: url, token });

test("status.get returns the stub modem model", async () => {
  const { data, error } = await sim.status.get({} as any);
  assert.equal(error, undefined);
  assert.ok(data);
  assert.equal(data!.modem.model, "STUB");
});

test("sms.send echoes the stub outbound", async () => {
  const { data, error } = await sim.sms.send({
    body: { to: "+12125551234", body: "hello" },
  } as any);
  assert.equal(error, undefined);
  assert.ok(data);
  assert.equal(data!.id, "01HSTUB");
  assert.equal(data!.state, "submitted");
});

test("callForwarding reads and updates carrier rules", async () => {
  const current = await sim.callForwarding.get({} as any);
  assert.equal(current.error, undefined);
  assert.equal(current.data!.items?.[1]?.number, "+12025550123");

  const updated = await sim.callForwarding.update({
    path: { reason: "unreachable" },
    body: { enabled: true, number: "+12025550199" },
  } as any);
  assert.equal(updated.error, undefined);
  assert.equal(updated.data!.enabled, true);
  assert.equal(updated.data!.number, "+12025550199");
});

test("missing auth fails", async () => {
  // Creating a new wrapper reconfigures the generated singleton used by its
  // service functions.
  const bad = new Sim7600Client({ baseUrl: url!, token: "wrong-token" });
  const { error } = await bad.status.get({} as any);
  assert.ok(error, "expected an error envelope");
  // The error envelope shape: {"error":{"code":"unauthorized","message":"..."}}
  const envelope = error as any;
  assert.equal(envelope.error.code, "unauthorized");
});

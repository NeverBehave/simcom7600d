// Public surface for @sim7600d/client.
// Re-exports the generated types and provides a lightly opinionated client
// wrapper that wires bearer auth and base URL once at construction.
//
// The generated fetch client is committed with the SDK, avoiding a runtime
// dependency on generator tooling.

import { client } from "./src/client.gen";

import {
  statusGet,
  callForwardingGet,
  callForwardingUpdate,
  smsSend,
  smsList,
  smsGet,
  smsDelete,
  callsList,
  callsDial,
  callsGet,
  callsAnswer,
  callsReject,
  callsHold,
  callsResume,
  callsMerge,
  callsHangup,
  callsDtmf,
  eventsList,
	adminCapabilities,
  adminReconcile,
  adminQueue,
  adminAt,
  adminAtReset,
  adminVacuum,
} from "./src/sdk.gen";

export type * from "./types";

export interface Sim7600ClientOptions {
  baseUrl: string;
  token: string;
  /** Override the fetch implementation (useful in tests / Node <18). */
  fetch?: typeof globalThis.fetch;
}

export class Sim7600Client {
  constructor(opts: Sim7600ClientOptions) {
    client.setConfig({
      baseUrl: opts.baseUrl,
      auth: opts.token,
      ...(opts.fetch ? { fetch: opts.fetch } : {}),
    });
  }

  status = {
    get: statusGet,
  };

  callForwarding = {
    get: callForwardingGet,
    update: callForwardingUpdate,
  };

  sms = {
    send: smsSend,
    list: smsList,
    get: smsGet,
    delete: smsDelete,
  };

  calls = {
    list: callsList,
    dial: callsDial,
    get: callsGet,
    answer: callsAnswer,
    reject: callsReject,
    hold: callsHold,
    resume: callsResume,
    merge: callsMerge,
    hangup: callsHangup,
    dtmf: callsDtmf,
  };

  events = {
    list: eventsList,
  };

  admin = {
	capabilities: adminCapabilities,
    reconcile: adminReconcile,
    queue: adminQueue,
    at: adminAt,
    atReset: adminAtReset,
    vacuum: adminVacuum,
  };
}

export function resetSim7600Client(baseUrl: string): void {
  client.setConfig({ baseUrl, auth: undefined });
}

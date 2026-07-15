package org.n4b5.sim7600.network

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import org.n4b5.sim7600.model.ServerConfig

class ApiClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: ApiClient

    @Before fun setUp() {
        server = MockWebServer().also { it.start() }
        client = ApiClient(ServerConfig(server.url("/").toString(), "secret-token"))
    }

    @After fun tearDown() { server.shutdown() }

    @Test fun statusUsesBearerAuthAndParsesSnapshot() = runBlocking {
        server.enqueue(MockResponse().setBody("""
            {"modem":{"model":"SIM7600G-H","imei":"123","firmware":"F1"},
             "sim":{"state":"ready","operator":"T-Mobile","imsi":"i","iccid":"c"},
             "network":{"registered":true,"tech":"LTE","band":"B2","rsrp_dbm":-91,"rsrq_db":-10,"csq":25},
             "battery":{"voltage_v":4.1},"uptime_s":90}
        """.trimIndent()))

        val status = client.status(refresh = true)
        val request = server.takeRequest()
        assertEquals("Bearer secret-token", request.getHeader("Authorization"))
        assertEquals("/v1/status?refresh=1", request.path)
        assertEquals("SIM7600G-H", status.model)
        assertEquals("T-Mobile", status.operator)
        assertEquals(-91, status.rsrpDbm)
    }

    @Test fun sendSmsAddsIdempotencyKeyAndJsonBody() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(202).setBody("""
            {"id":"sms-1","direction":"out","to":"+15551234567","body":"hello","state":"submitted","parts":[{"mr":1}],"encoding":"gsm7","ts":"2026-07-14T00:00:00Z"}
        """.trimIndent()))

        val message = client.sendSms("+15551234567", "hello")
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/v1/sms", request.path)
        assertNotNull(request.getHeader("Idempotency-Key"))
        val requestBody = org.json.JSONObject(request.body.readUtf8())
        assertEquals("hello", requestBody.getString("body"))
        assertFalse(requestBody.has("delivery_report"))
        assertFalse(message.inbound)
    }

    @Test fun callAndForwardingActionsUseExpectedRoutes() = runBlocking {
        server.enqueue(MockResponse().setBody("""
            {"id":"call-1","direction":"in","from":"+15550000000","state":"active","started_at":"2026-07-14T00:00:00Z"}
        """.trimIndent()))
        client.callAction("call-1", "answer")
        assertEquals("/v1/calls/call-1/answer", server.takeRequest().path)

        server.enqueue(MockResponse().setBody("""
            {"reason":"no_reply","enabled":true,"number":"+15551112222","timeout_seconds":25,"available":true}
        """.trimIndent()))
        client.updateForwarding("no_reply", true, "+15551112222", 25)
        val forwarding = server.takeRequest()
        assertEquals("PUT", forwarding.method)
        assertEquals("/v1/call-forwarding/no_reply", forwarding.path)
        assertEquals(25, org.json.JSONObject(forwarding.body.readUtf8()).getInt("timeout_seconds"))
    }

    @Test fun eventStreamResumesFromStoredCursor() {
        val request = client.eventStreamCall(73).request()
        assertEquals("/v1/events/stream?since=73", request.url.encodedPath + "?" + request.url.encodedQuery)
        assertEquals("Bearer secret-token", request.header("Authorization"))
    }

    @Test fun cloudflareGatewayErrorIncludesUsefulSmsDiagnostics() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(502)
                .addHeader("Server", "cloudflare")
                .addHeader("CF-Ray", "abc123-SJC")
                .addHeader("Content-Type", "text/html")
                .setBody("<html><title>Bad gateway</title></html>"),
        )

        try {
            client.sendSms("+15551234567", "hello")
            fail("expected ApiException")
        } catch (error: ApiException) {
            assertEquals(502, error.status)
            assertEquals("POST", error.method)
            assertEquals("/v1/sms", error.route)
            assertEquals("abc123-SJC", error.cfRay)
            assertTrue(error.message.orEmpty().contains("Cloudflare could not get a complete response"))
            assertTrue(error.message.orEmpty().contains("refresh the conversation"))
        }
    }
}

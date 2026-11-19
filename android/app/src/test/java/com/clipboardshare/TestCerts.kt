package com.clipboardshare

import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate

/** Two self-signed ECDSA P-256 "CN=Nudge" certificates made with openssl for the tests. */
object TestCerts {
    private const val A_PEM = """-----BEGIN CERTIFICATE-----
MIIBdTCCARugAwIBAgIUfm0i0dA0Rh+CFF0E0H3ZFUNcfL4wCgYIKoZIzj0EAwIw
EDEOMAwGA1UEAwwFTnVkZ2UwHhcNMjYxMTE5MjEzNjIwWhcNNDYxMTE0MjEzNjIw
WjAQMQ4wDAYDVQQDDAVOdWRnZTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABPDs
MKYeznfSo+UVif9hcjukc2Zn4wnBBFNC+1XUOfN4sZ2p7C4lfVSrzfgiKppylfvZ
T+kW4t5wg4BzEOz3ct+jUzBRMB0GA1UdDgQWBBQFzUQb22Ei8Bwr6su1MRfMMB5y
CDAfBgNVHSMEGDAWgBQFzUQb22Ei8Bwr6su1MRfMMB5yCDAPBgNVHRMBAf8EBTAD
AQH/MAoGCCqGSM49BAMCA0gAMEUCIQCWSwA8beGnbj2slC/kpCwFO12QAvpLCatD
g+FMrAGu8QIgWaBuiDbXUnq9LnXfgJ+FQKwXBqI4KeT92synJUIA928=
-----END CERTIFICATE-----"""

    private const val B_PEM = """-----BEGIN CERTIFICATE-----
MIIBdTCCARugAwIBAgIUX6oFkDyGP37IzX1eY3ib2KWTYR4wCgYIKoZIzj0EAwIw
EDEOMAwGA1UEAwwFTnVkZ2UwHhcNMjYxMTE5MjEzNjIwWhcNNDYxMTE0MjEzNjIw
WjAQMQ4wDAYDVQQDDAVOdWRnZTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABPrI
oNCpWhEmYL+tKFTD0+NXle2RmEeAQSgL9SVR8nCuN6GLyJ735AKgXvJandVJjQ/7
PIuED+MxKy67UYi8Dx6jUzBRMB0GA1UdDgQWBBTcPw5m4jJT5UxQGnv9Og2Clyw2
yTAfBgNVHSMEGDAWgBTcPw5m4jJT5UxQGnv9Og2Clyw2yTAPBgNVHRMBAf8EBTAD
AQH/MAoGCCqGSM49BAMCA0gAMEUCIQDnXsoEfnP358SZGsYBe3g5xHkRCqe9Zewy
mMOjrEJaPwIgFSsC4KG1qPt+41LhRDk8VQ6yWP7ekhh2PFx3dFm/60A=
-----END CERTIFICATE-----"""

    const val A_FINGERPRINT = "527af0c206b28bf397b7baefba56d2e8aaa8b6907adc6f0396413ce8a0c54cf2"
    const val B_FINGERPRINT = "b711b9ca977578fa35642022d2460e26ac759b4a91b0c45bf4d3d6b086a2b2c7"

    val a: X509Certificate by lazy { parse(A_PEM) }
    val b: X509Certificate by lazy { parse(B_PEM) }

    private fun parse(pem: String): X509Certificate =
        CertificateFactory.getInstance("X.509")
            .generateCertificate(pem.byteInputStream()) as X509Certificate
}

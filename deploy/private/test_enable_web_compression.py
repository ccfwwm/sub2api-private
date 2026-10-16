import unittest

from enable_web_compression import INCLUDE, add_include


class WebCompressionTest(unittest.TestCase):
    def test_preserves_http_other_hosts_and_streaming(self):
        plain = 'server {\n listen 80;\n server_name code.aicodeme.cn;\n return 301 https://$host$request_uri;\n}\n'
        tls = 'server {\n listen 443 ssl http2;\n server_name code.aicodeme.cn;\n location ^~ /v1/ { gzip off; proxy_buffering off; }\n}\n'
        other = 'server {\n listen 443 ssl;\n server_name unrelated.example;\n location / { return 404; }\n}\n'
        updated = add_include(plain + tls + other, 'code.aicodeme.cn')
        self.assertTrue(updated.startswith(plain))
        self.assertTrue(updated.endswith(other))
        self.assertIn('location ^~ /v1/ { gzip off; proxy_buffering off; }', updated)
        self.assertEqual(updated.count(INCLUDE), 1)
        self.assertEqual(add_include(updated, 'code.aicodeme.cn'), updated)

    def test_missing_https_or_hostname_is_rejected(self):
        for content in ['server {\n listen 80;\n server_name code.aicodeme.cn;\n}\n',
                        'server {\n listen 443 ssl;\n server_name unrelated.example;\n}\n']:
            with self.assertRaises(ValueError):
                add_include(content, 'code.aicodeme.cn')

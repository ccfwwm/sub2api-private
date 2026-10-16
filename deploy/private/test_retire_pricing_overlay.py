import unittest

from retire_pricing_overlay import compose_without_overlay


class NativePricingCutoverTests(unittest.TestCase):
    def test_removes_only_overlay_and_preserves_other_services(self):
        source = '''services:
  sub2api:
    environment:
      - PRICING_REMOTE_URL=http://pricing-overlay:8090/model-pricing.json
      - PRICING_HASH_URL=http://pricing-overlay:8090/model-pricing.sha256
      - OTHER=keep
    depends_on:
      pricing-overlay:
        condition: service_healthy
      postgres:
        condition: service_healthy
  pricing-overlay:
    container_name: sub2api-pricing-overlay
    command: [python, /app/pricing_overlay.py]
  postgres:
    image: postgres:18-alpine
networks:
  existing:
'''
        result = compose_without_overlay(source)
        self.assertNotIn('pricing-overlay', result)
        self.assertIn('PRICING_OVERRIDE_FILE=/app/data/pricing/domestic-pricing-overrides.json', result)
        self.assertIn('      - OTHER=keep', result)
        self.assertIn('      postgres:\n        condition: service_healthy', result)
        self.assertIn('  postgres:\n    image: postgres:18-alpine', result)
        self.assertIn('networks:\n  existing:', result)

    def test_unknown_or_already_modified_configuration_is_rejected(self):
        with self.assertRaisesRegex(ValueError, 'unexpected'):
            compose_without_overlay('services:\n  sub2api:\n    image: unknown\n')


if __name__ == '__main__':
    unittest.main()

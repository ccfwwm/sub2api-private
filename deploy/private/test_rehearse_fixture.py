import unittest

from rehearse_fixture import validate_docs_content


class DocumentationFixtureTests(unittest.TestCase):
    def setUp(self):
        self.html = '<title>词元互联 · 接口文档</title>'
        self.knowledge = [{'id': 'cc-switch-config'}, {'id': 'api-404-html'}]
        self.legacy = {'code': 0, 'data': {
            'slug': 'imagevideo',
            'version': 15,
            'content': [{'id': 'gpt-edit'}],
        }}

    def test_accepts_configuration_articles_and_compatible_legacy_response(self):
        validate_docs_content(self.html, self.knowledge, self.legacy)

    def test_requires_configuration_articles(self):
        with self.assertRaisesRegex(ValueError, 'configuration documentation'):
            validate_docs_content(self.html, [{'id': 'cc-switch-config'}], self.legacy)

    def test_preserves_the_legacy_public_document_contract(self):
        legacy = {'code': 0, 'data': {'slug': 'renamed', 'content': [{'id': 'gpt-edit'}]}}
        with self.assertRaisesRegex(ValueError, 'legacy documentation contract'):
            validate_docs_content(self.html, self.knowledge, legacy)

    def test_requires_the_rebuilt_portal_title(self):
        with self.assertRaisesRegex(ValueError, 'documentation route'):
            validate_docs_content('<title>Old title</title>', self.knowledge, self.legacy)


if __name__ == '__main__':
    unittest.main()

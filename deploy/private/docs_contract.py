"""Shared acceptance checks for the standalone public documentation service."""


def validate_docs_portal_html(html):
    titles = (
        '<title>文档 · 词元互联</title>',
        '<title>词元互联 · 接口文档</title>',
    )
    if not any(title in html for title in titles):
        raise ValueError('documentation route content check failed')


def validate_docs_contract(html, knowledge, legacy):
    validate_docs_portal_html(html)
    knowledge_ids = {article.get('id') for article in knowledge if isinstance(article, dict)}
    if not {'cc-switch-config', 'api-404-html'} <= knowledge_ids:
        raise ValueError('configuration documentation check failed')
    legacy_data = legacy.get('data') if isinstance(legacy, dict) else None
    if not isinstance(legacy_data, dict) or legacy_data.get('slug') != 'imagevideo' or not any(
        isinstance(section, dict) and section.get('id') == 'gpt-edit'
        for section in legacy_data.get('content', [])
    ):
        raise ValueError('legacy documentation contract check failed')

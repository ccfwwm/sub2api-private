-- Pin only protocol templates identified by previously saved protocol fields.
-- Providers, base URLs, model names, and model prefixes are deliberately ignored.
WITH legacy_models AS (
    SELECT a.id, capability.item, capability.ordinality,
        CASE
            WHEN capability.item->>'request_format' = 'v6k_url'
                AND capability.item->>'media_type' = 'video'
                AND COALESCE(capability.item->'operations', '[]'::jsonb) @> '["video"]'::jsonb
                THEN 'video-arrays'
            WHEN capability.item->>'request_format' = 'yunfei_multimodal'
                AND capability.item->>'media_type' = 'video'
                AND COALESCE(capability.item->'operations', '[]'::jsonb) @> '["video"]'::jsonb
                THEN 'video-reference-arrays'
            WHEN COALESCE(capability.item->>'request_format', '') IN ('', 'legacy') THEN
                CASE capability.item->>'adapter'
                    WHEN 'v6k_video_gateway' THEN
                        CASE WHEN capability.item->>'media_type' = 'video'
                            AND COALESCE(capability.item->'operations', '[]'::jsonb) @> '["video"]'::jsonb
                            THEN 'video-arrays' ELSE '' END
                    WHEN 'yunfei_omni' THEN
                        CASE WHEN capability.item->>'media_type' = 'video'
                            AND COALESCE(capability.item->'operations', '[]'::jsonb) @> '["video"]'::jsonb
                            THEN 'video-arrays' ELSE '' END
                    WHEN 'yunfei_grok' THEN
                        CASE WHEN capability.item->>'media_type' = 'video'
                            AND COALESCE(capability.item->'operations', '[]'::jsonb) @> '["video"]'::jsonb
                            THEN 'video-single-image' ELSE '' END
                    WHEN 'ark_native' THEN
                        CASE WHEN capability.item->>'media_type' = 'video'
                            AND COALESCE(capability.item->'operations', '[]'::jsonb) @> '["video"]'::jsonb
                            THEN 'media-content-blocks' ELSE '' END
                    WHEN 'yunfei_gpt_images' THEN
                        CASE WHEN capability.item->>'media_type' = 'image'
                            AND (COALESCE(capability.item->'operations', '[]'::jsonb) @> '["image"]'::jsonb
                                OR COALESCE(capability.item->'operations', '[]'::jsonb) @> '["image_edit"]'::jsonb)
                            THEN CASE WHEN COALESCE(capability.item->'operations', '[]'::jsonb) @> '["image_edit"]'::jsonb
                                THEN 'image-json-multipart' ELSE 'image-json' END
                            ELSE '' END
                    WHEN 'yunfei_gemini_native' THEN
                        CASE WHEN capability.item->>'media_type' IN ('image', 'audio')
                            AND (COALESCE(capability.item->'operations', '[]'::jsonb) @> '["gemini_image"]'::jsonb
                                OR COALESCE(capability.item->'operations', '[]'::jsonb) @> '["image"]'::jsonb
                                OR COALESCE(capability.item->'operations', '[]'::jsonb) @> '["audio"]'::jsonb)
                            THEN 'gemini-parts' ELSE '' END
                    WHEN 'yunfei_lyria' THEN
                        CASE WHEN capability.item->>'media_type' = 'audio'
                            AND (COALESCE(capability.item->'operations', '[]'::jsonb) @> '["audio"]'::jsonb
                                OR COALESCE(capability.item->'operations', '[]'::jsonb) @> '["chat_music"]'::jsonb)
                            THEN 'music-chat-json' ELSE '' END
                    ELSE ''
                END
            ELSE ''
        END AS template_id
    FROM accounts a
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE WHEN jsonb_typeof(a.extra->'media_models') = 'array'
            THEN a.extra->'media_models' ELSE '[]'::jsonb END
    ) WITH ORDINALITY AS capability(item, ordinality)
    WHERE a.platform = 'imagevideo'
      AND jsonb_typeof(a.extra->'media_models') = 'array'
      AND a.extra->'media_models' <> '[]'::jsonb
), migrated AS (
    SELECT id, jsonb_agg(
        CASE
            WHEN jsonb_typeof(item->'template_ref') = 'object'
                AND COALESCE(item #>> '{template_ref,id}', '') <> ''
                AND COALESCE(item #>> '{template_ref,version}', '') ~ '^[1-9][0-9]*$'
                THEN item
            WHEN item ? 'template_ref' THEN jsonb_set(
                jsonb_set(item, '{enabled}', 'false'::jsonb, true),
                '{validation_status}',
                to_jsonb('待配置协议模板'::text),
                true
            )
            WHEN template_id <> '' THEN jsonb_set(
                item,
                '{template_ref}',
                jsonb_build_object('id', template_id, 'version', 1),
                true
            )
            ELSE jsonb_set(
                jsonb_set(item, '{enabled}', 'false'::jsonb, true),
                '{validation_status}',
                to_jsonb('待配置协议模板'::text),
                true
            )
        END ORDER BY ordinality
    ) AS models
    FROM legacy_models
    GROUP BY id
)
UPDATE accounts a
SET extra = jsonb_set(COALESCE(a.extra, '{}'::jsonb), '{media_models}', migrated.models, true)
FROM migrated
WHERE a.id = migrated.id;

import base64
import hashlib
import json
import tempfile
from pathlib import Path
import unittest
from unittest.mock import patch
import sys
if sys.platform == 'win32':
 import types
 sys.modules['fcntl']=types.SimpleNamespace(flock=lambda *args:None,LOCK_EX=1,LOCK_NB=2)
import auto_deploy
from auto_deploy import check_order, dispatch, release_assets, stage_reviewed_migrations, REVIEWED_MIGRATION_ASSETS
from controller import atomic_json

class AutomaticDeployTests(unittest.TestCase):
 def test_public_image_pull_isolates_host_credentials_without_changing_them(self):
  image='ghcr.io/ccfwwm/sub2api-private@sha256:'+'a'*64
  configs=[]
  def command(args):
   self.assertEqual(args[:2],['docker','--config'])
   self.assertEqual(args[3:],['pull',image])
   config=Path(args[2]); configs.append(config)
   self.assertTrue(config.is_dir())
   self.assertFalse((config/'config.json').exists())
  with patch('auto_deploy.run',side_effect=command):
   auto_deploy.pull_public_image(image)
  self.assertFalse(configs[0].exists())

 def test_stage_missing_historical_assets_from_exact_commit_and_verify_checksum(self):
  name='242_group_i18n.sql'; sql=b'ALTER TABLE groups ADD COLUMN fixture integer;\n'
  manifest={'commit':'a'*40,'migrations':{name:hashlib.sha256(sql.strip()).hexdigest()}}
  with tempfile.TemporaryDirectory() as root:
   assets,release=Path(root)/'assets',Path(root)/'release';assets.mkdir();release.mkdir()
   atomic_json(assets/'deployment.json',manifest)
   with patch('auto_deploy.run',return_value=json.dumps({'content':base64.b64encode(sql).decode()}).encode()) as command:
    self.assertEqual(stage_reviewed_migrations(assets,release),[name])
   self.assertIn('?ref='+'a'*40,command.call_args.args[0][-1])
   self.assertEqual((release/name).read_bytes(),sql)
   (assets/name).write_bytes(sql+b'DROP TABLE users;')
   with self.assertRaisesRegex(ValueError,'checksum'):
    stage_reviewed_migrations(assets,release)

 def test_publishing_cannot_call_production_workflow_and_manual_deploy_selects_cutover_mode(self):
  root=Path(__file__).resolve().parents[2]
  release=(root/'.github/workflows/release.yml').read_text(encoding='utf-8')
  deploy=(root/'.github/workflows/deploy-production.yml').read_text(encoding='utf-8')
  source=(root/'deploy/private/auto_deploy.py').read_text(encoding='utf-8')
  self.assertNotIn('uses: ./.github/workflows/deploy-production.yml',release)
  self.assertNotIn('  workflow_call:',deploy)
  self.assertIn('  workflow_dispatch:',deploy)
  self.assertIn("'immediate':job.get('immediate', False)",source)
  self.assertIn("'immediate':len(words) == 3",source)
  self.assertIn('      immediate:',deploy)
  self.assertIn('DEPLOY_COMMAND+=" --immediate"',deploy)
 def test_release_workflows_publish_every_reviewed_upstream_migration_asset(self):
  root=Path(__file__).resolve().parents[2]
  upstream=('242_group_i18n.sql','243_gemini_cached_contents.sql',
   '244_gemini_cached_contents_channel_usage.sql','245_gemini_cached_contents_mutation_lease.sql',
   '246_enable_gemini_image_generation_groups.sql')
  release=(root/'.github/workflows/release.yml').read_text(encoding='utf-8')
  finalize=(root/'.github/workflows/finalize-private-release.yml').read_text(encoding='utf-8')
  release_inventory=release[release.index('sha256sum deployment.json'):release.index('git rev-parse HEAD > management-source.txt')]
  finalize_inventory=finalize[finalize.index('sha256sum deployment.json'):finalize.index('sha256sum server-management.tar.gz')]
  for name in upstream:
   with self.subTest(name=name):
    self.assertIn(name,release_inventory)
    self.assertIn(name,finalize_inventory)
    self.assertIn(name,release[release.index('      - uses: actions/upload-artifact'):])
  self.assertIn('242_group_i18n.sql',finalize[finalize.index('gh release create'):])
 def test_release_assets_downloads_docs_receipt_before_checksum_validation(self):
  version='v1.2.7'; commit='a'*40; docs_name='docs/multimodal/CHANNEL_VERIFICATION_CURRENT.json'
  verification=json.dumps({'version':version,'status':'recorded'}).encode()
  manifest={
   'schema':1,'repository':'ccfwwm/sub2api-private','image':'ghcr.io/ccfwwm/sub2api-private',
   'platform':'linux/amd64','version':version,'digest':'sha256:'+'b'*64,'commit':commit,
   'upstream_commit':'c'*40,'upstream_version':'0.2.15','migrations':{'248_public_imagevideo_docs.sql':'d'*64},
   'docs_image':'ghcr.io/ccfwwm/sub2api-private-docs','docs_digest':'sha256:'+'e'*64,
   'channel_verification_record':{'path':docs_name,'sha256':hashlib.sha256(verification).hexdigest()}}
  asset_bytes={
   'deployment.json':(json.dumps(manifest)+'\n').encode(),
   'docs-image-inspect.json':b'[{"Config":{"Labels":{}}}]\n',
   'server-management.tar.gz':b'fixture archive',
   'management-source.txt':(commit+'\n').encode(),
   '248_public_imagevideo_docs.sql':b'CREATE TABLE fixture_docs (id integer);\n'}
  def checksum(name): return hashlib.sha256(asset_bytes[name]).hexdigest()
  asset_bytes['checksums.txt']=(''.join(f'{checksum(n)}  {n}\n' for n in
   ['deployment.json','docs-image-inspect.json','248_public_imagevideo_docs.sql'])).encode()
  asset_bytes['management-checksums.txt']=(''.join(f'{checksum(n)}  {n}\n' for n in
   ['server-management.tar.gz','management-source.txt'])).encode()
  names=list(asset_bytes)
  release={'draft':False,'prerelease':False,'tag_name':version,'assets':[{'name':n} for n in names]}
  downloads=[]
  with tempfile.TemporaryDirectory() as root:
   folder=Path(root); folder.mkdir(exist_ok=True)
   def fake_run(args, **kwargs):
    if args[:2]==['gh','api']:
     path=args[2]
     if path.endswith('/releases/tags/'+version): return json.dumps(release).encode()
     if path.startswith('repos/ccfwwm/sub2api-private/contents/'):
      return json.dumps({'content':base64.b64encode(verification).decode()}).encode()
     if path.endswith('/commits/'+version): return json.dumps({'sha':commit}).encode()
     if path.endswith('/compare/'+commit+'...main'): return json.dumps({'status':'identical'}).encode()
    if args[:3]==['gh','release','download']:
     selected=[args[i+1] for i,v in enumerate(args[:-1]) if v=='--pattern']
     downloads.append(selected)
     for name in selected: (folder/name).write_bytes(asset_bytes[name])
     return b''
    raise AssertionError(args)
   with patch('auto_deploy.run',side_effect=fake_run):
    result=release_assets(version,folder)
   self.assertEqual(result,manifest)
   self.assertIn(['docs-image-inspect.json'],downloads)
   self.assertTrue((folder/'docs-image-inspect.json').is_file())

 def test_reviewed_migration_staging_includes_public_docs_asset(self):
  with tempfile.TemporaryDirectory() as root:
   assets, release = Path(root) / 'assets', Path(root) / 'release'
   assets.mkdir(); release.mkdir()
   for name in REVIEWED_MIGRATION_ASSETS:
    (assets / name).write_text(name)
   staged = stage_reviewed_migrations(assets, release)
   self.assertEqual(staged, list(REVIEWED_MIGRATION_ASSETS))
   self.assertEqual((release / '248_public_imagevideo_docs.sql').read_text(), '248_public_imagevideo_docs.sql')
   for name in ('249_media_protocol_templates.sql', '250_migrate_legacy_media_template_refs.sql', '251_media_group_availability.sql'):
    self.assertEqual((release / name).read_text(), name)

 def test_online_verification_requires_all_production_domains(self):
  with patch('auto_deploy.run',return_value=b'{"data":{"version":"1.2.1"}}') as run,patch('auto_deploy.inspect',return_value={'Image':'fixed-image'}):
   auto_deploy.verify_online('v1.2.1','fixed-image')
  checked=[call.args[0][-1] for call in run.call_args_list]
  for base in ['http://127.0.0.1:8081','https://code.aicodeme.cn','https://code.aicodeme.xyz','https://hkcode.aicodeme.xyz']:
   self.assertIn(base+'/health',checked)
   self.assertIn(base+'/api/v1/settings/public',checked)
 def test_older_release_rejected(self):
  with self.assertRaisesRegex(ValueError,'older'):check_order('v1.1.7','v1.2.0')
  check_order('v1.2.0','v1.2.0');check_order('v1.3.0','v1.2.0')
 def test_ssh_no_arbitrary_commands(self):
  for words in [['bash','-c','id'],['deploy','v1.2.0;id'],['deploy','v1.2.0-rc'],['status','../state'],['deploy','v1.2.0','extra'],['deploy','v1.2.0','--immediate;id']]:
   with tempfile.TemporaryDirectory() as root,patch.object(auto_deploy,'AUTO',Path(root)):
    with self.assertRaises(ValueError):dispatch(words)
 def test_duplicate_reuses_durable_job(self):
  with tempfile.TemporaryDirectory() as root,patch.object(auto_deploy,'AUTO',Path(root)),patch('auto_deploy.subprocess.run') as run:
   job={'id':'a'*32,'version':'v1.2.0','status':'running'}
   atomic_json(Path(root)/(job['id']+'.json'),job)
   atomic_json(Path(root)/'deployed.json',{'version':'v1.1.7'})
   self.assertEqual(dispatch(['deploy','v1.2.0']),job);run.assert_not_called()
 def test_immediate_deploy_is_explicit_and_persisted_in_restricted_job(self):
  with tempfile.TemporaryDirectory() as root,patch.object(auto_deploy,'AUTO',Path(root)),patch('auto_deploy.subprocess.run') as run:
   atomic_json(Path(root)/'deployed.json',{'version':'v1.3.15'})
   job=dispatch(['deploy','v1.3.16','--immediate'])
   self.assertTrue(job['immediate'])
   self.assertTrue(run.called)
   saved=json.loads((Path(root)/(job['id']+'.json')).read_text())
   self.assertTrue(saved['immediate'])
 def test_immediate_skips_drain_but_retains_health_and_protection(self):
  from manage import Manager
  with tempfile.TemporaryDirectory() as root:
   manager=Manager(Path(root)/'state',Path(root)/'production')
   target={'services':{'sub2api':{'environment':{}}}}
   job={'id':'a'*32,'immediate':True}
   state={'Id':'fixed','Image':'image','State':{'StartedAt':'fixed'}}
   with patch('manage.inspect',return_value=state),patch.object(manager.c,'inspect',return_value=state),patch.object(manager,'drain') as drain,patch.object(manager,'switch') as switch,patch.object(manager.c,'wait_command'),patch('manage.run',return_value=b'{"data":{"version":"1.2.0"}}'):
    manager.cutover(job,target,{},'v1.2.0')
   drain.assert_not_called();switch.assert_called_once();self.assertEqual(job['status'],'succeeded')
   self.assertEqual(target['services']['sub2api']['stop_grace_period'],'1s')
   self.assertEqual(set(job['protected_containers']), {'sub2api-postgres', 'sub2api-redis'})
if __name__=='__main__': unittest.main()

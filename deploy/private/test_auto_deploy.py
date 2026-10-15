import tempfile
from pathlib import Path
import unittest
from unittest.mock import patch
import sys
if sys.platform == 'win32':
 import types
 sys.modules['fcntl']=types.SimpleNamespace(flock=lambda *args:None,LOCK_EX=1,LOCK_NB=2)
import auto_deploy
from auto_deploy import check_order, dispatch, stage_reviewed_migrations, REVIEWED_MIGRATION_ASSETS
from controller import atomic_json

class AutomaticDeployTests(unittest.TestCase):
 def test_reviewed_migration_staging_includes_public_docs_asset(self):
  with tempfile.TemporaryDirectory() as root:
   assets, release = Path(root) / 'assets', Path(root) / 'release'
   assets.mkdir(); release.mkdir()
   for name in REVIEWED_MIGRATION_ASSETS:
    (assets / name).write_text(name)
   staged = stage_reviewed_migrations(assets, release)
   self.assertEqual(staged, list(REVIEWED_MIGRATION_ASSETS))
   self.assertEqual((release / '248_public_imagevideo_docs.sql').read_text(), '248_public_imagevideo_docs.sql')

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
  for words in [['bash','-c','id'],['deploy','v1.2.0;id'],['deploy','v1.2.0-rc'],['status','../state'],['deploy','v1.2.0','extra']]:
   with tempfile.TemporaryDirectory() as root,patch.object(auto_deploy,'AUTO',Path(root)):
    with self.assertRaises(ValueError):dispatch(words)
 def test_duplicate_reuses_durable_job(self):
  with tempfile.TemporaryDirectory() as root,patch.object(auto_deploy,'AUTO',Path(root)),patch('auto_deploy.subprocess.run') as run:
   job={'id':'a'*32,'version':'v1.2.0','status':'running'}
   atomic_json(Path(root)/(job['id']+'.json'),job)
   atomic_json(Path(root)/'deployed.json',{'version':'v1.1.7'})
   self.assertEqual(dispatch(['deploy','v1.2.0']),job);run.assert_not_called()
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
   self.assertIn('sub2api-pricing-overlay',job['protected_containers'])
if __name__=='__main__': unittest.main()

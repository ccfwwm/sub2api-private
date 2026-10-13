import {describe,it,expect} from 'vitest'
import {accountMediaCapabilities,newMediaCapability} from '@/types/media'

describe('Unified media presets',()=>{
 it('loads Omni and Veo while unknown models require an explicit interface',()=>{
  expect(newMediaCapability('omni-flash')).toMatchObject({adapter:'yunfei_omni',enabled:false,duration:{values:[4,6,8,10]}})
  expect(newMediaCapability('veo-3.1-fast').duration.values).toEqual([8])
  expect(newMediaCapability('unknown').adapter).toBe('')
 })
 it('preserves removal and legacy aliases',()=>{
  expect(accountMediaCapabilities({media_models:[]},{model_mapping:{old:'omni-flash'}})).toEqual([])
  expect(accountMediaCapabilities({}, {model_mapping:{alias:'omni-flash'}})[0]).toMatchObject({model:'alias',upstream_model:'omni-flash',adapter:'yunfei_omni'})
 })
 it('repairs only placeholders and preserves enabled flag and overrides',()=>{
  const cap=newMediaCapability('unknown');cap.model=cap.upstream_model='omni-flash';cap.adapter='v6k_video_gateway';cap.media_type='video';cap.enabled=true;cap.limits.max_concurrent_tasks=7
  const [updated]=accountMediaCapabilities({media_models:[cap]}, {base_url:'https://img.yunfei.best'})
  expect(updated.adapter).toBe('yunfei_omni');expect(updated.enabled).toBe(true);expect(updated.limits.max_concurrent_tasks).toBe(7)
  cap.duration.default=30
  expect(accountMediaCapabilities({media_models:[cap]},{base_url:'https://img.yunfei.best'})[0].adapter).toBe('v6k_video_gateway')
 })
})

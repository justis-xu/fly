import type { NextConfig } from 'next';
import {sourceHash} from './scripts/source-hash.mjs';
const config: NextConfig = {
  output: 'export',
  generateBuildId:async()=>sourceHash(),
  images: { unoptimized: true },
  poweredByHeader: false,
};
export default config;

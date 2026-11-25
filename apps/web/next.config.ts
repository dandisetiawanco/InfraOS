import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  transpilePackages: ["@infraos/contracts"],
  images: {
    unoptimized: true,
  },
};

export default nextConfig;

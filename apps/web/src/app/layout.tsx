import type { Metadata, Viewport } from "next";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
import { Providers } from "@/components/providers";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "InfraOS | Infrastructure under control.",
    template: "%s | InfraOS",
  },
  description:
    "One workspace to monitor, deploy, and operate your infrastructure.",
  openGraph: {
    title: "InfraOS | Infrastructure under control.",
    description:
      "Monitor your systems, understand operational events, and manage deployments from one connected workspace.",
    images: [{ url: "/brand/og-image.svg", width: 1200, height: 630, alt: "InfraOS overview" }],
  },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
};

const themeInitScript = `try{var s=localStorage.getItem("color-scheme");if(s==="light"||s==="dark"){document.querySelector('meta[name="color-scheme"]').setAttribute("content",s);}}catch(e){}`;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${GeistSans.variable} ${GeistMono.variable}`}>
      <head>
        <meta name="color-scheme" content="light dark" />
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
      </head>
      <body className="infra-canvas antialiased">
        <a
          href="#main-content"
          className="visually-hidden fixed left-3 top-3 z-50 rounded-lg bg-[var(--accent)] px-4 py-2 text-sm font-semibold text-[var(--accent-contrast)]"
        >
          Skip to main content
        </a>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}

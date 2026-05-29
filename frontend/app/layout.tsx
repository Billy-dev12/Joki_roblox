import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Joki Roblox — Blox Fruit Pro",
  description: "Cek status joki Roblox kamu secara real-time. Pull Lever & Upgrade Race V4 Blox Fruit.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="id">
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
      </head>
      <body>
        {/* Aurora Background — always present */}
        <div className="aurora-wrap" aria-hidden>
          <div className="orb orb-1" />
          <div className="orb orb-2" />
          <div className="orb orb-3" />
        </div>
        {children}
      </body>
    </html>
  );
}

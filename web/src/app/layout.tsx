import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Hatch - A lightweight microVM orchestrator designed for agnatic workloads",
  description: "Hatch is a wrapper around Firecracker for spnning up microVMs, designed for agentic workloads, with a REST api for lifecycle mangement, wake-on-request, snapshot/restore for idel VMs and subdomain-based reverse proxy.",
  openGraph: {
    title: "Hatch - A lightweight microVM orchestrator designed for agnatic workloads",
    description: "Hatch is a wrapper around Firecracker for spnning up microVMs, designed for agentic workloads, with a REST api for lifecycle mangement, wake-on-request, snapshot/restore for idel VMs and subdomain-based reverse proxy.",
    url: "https://hatchvm.com",
    siteName: "Hatch",
    images: [
      {
        url: "https://hatchvm.com/og/ogImage.png",
        width: 1200,
        height: 630,
        alt: "Hatch – microVM orchestration",
      },
    ],
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "Hatch",
    description: "Lightweight microVM orchestration for AI agents.",
    images: ["https://hatchvm.com/og/ogImage.png"],
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className="dark">
      <body
        className={`${geistSans.variable} ${geistMono.variable} bg-black text-zinc-100 antialiased`}
      >
        {children}
      </body>
    </html>
  );
}

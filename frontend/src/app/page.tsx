import { redirect } from "next/navigation";

import { ROUTES } from "@/lib/routes";

export default function RootPage() {
  redirect(ROUTES.dashboard);
}

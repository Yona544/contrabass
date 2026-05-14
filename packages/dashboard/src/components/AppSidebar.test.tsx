import { afterEach, describe, expect, it } from "bun:test";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SidebarProvider } from "@/components/ui/sidebar";
import { AppSidebar, type ViewId } from "./AppSidebar";

afterEach(() => {
  cleanup();
});

describe("AppSidebar", () => {
  it("selects settings as a real navigation view", () => {
    const selections: ViewId[] = [];

    render(
      <TooltipProvider>
        <SidebarProvider>
          <AppSidebar
            active="settings"
            onSelect={(id) => {
              selections.push(id);
            }}
            counts={{ running: 1 }}
            connected={true}
            runtimeLabel="5分钟"
          />
        </SidebarProvider>
      </TooltipProvider>,
    );

    const settings = screen.getByRole("button", { name: "设置" });
    (expect(settings) as any).toHaveAttribute("data-active", "true");

    fireEvent.click(settings);

    expect(selections).toEqual(["settings"]);
  });
});

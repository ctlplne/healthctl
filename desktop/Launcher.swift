import AppKit
import UniformTypeIdentifiers

final class Launcher: NSObject, NSApplicationDelegate {
    private let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 540, height: 410),
                                  styleMask: [.titled, .closable, .miniaturizable], backing: .buffered, defer: false)
    private let status = NSTextField(wrappingLabelWithString: "Your workspace runs on this Mac using Docker Desktop.")
    private let stack = NSStackView()
    private var buttons: [NSButton] = []
    private var working = false

    func applicationDidFinishLaunching(_ notification: Notification) {
        window.title = "healthctl · a ctlplne studio product"
        let heading = NSTextField(labelWithString: "healthctl")
        heading.font = NSFont.systemFont(ofSize: 29, weight: .semibold)
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 12
        stack.addArrangedSubview(heading)
        guard let url = Bundle.main.url(forResource: "preview", withExtension: "txt"),
              let preview = try? String(contentsOf: url, encoding: .utf8) else {
            let alert = NSAlert()
            alert.messageText = "The beta notice is missing. Download healthctl again."
            alert.runModal()
            NSApp.terminate(nil)
            return
        }
        stack.addArrangedSubview(NSTextField(wrappingLabelWithString: preview.trimmingCharacters(in: .whitespacesAndNewlines)))
        for (label, action) in [("Open journal or try the demo", #selector(openJournal)), ("Connect a model", #selector(settings)),
                                ("Set or clear an OpenAI API key…", #selector(platformKey)), ("Back up workspace…", #selector(backup)), ("Restore a backup into a new workspace…", #selector(restore)),
                                ("Stop the workspace", #selector(stop)), ("Save logs…", #selector(logs)),
                                ("Delete this workspace…", #selector(deleteWorkspace)), ("Remove all healthctl data…", #selector(uninstall)),
                                ("Get Docker Desktop", #selector(dockerLink))] {
            let button = NSButton(title: label, target: self, action: action)
            buttons.append(button)
            stack.addArrangedSubview(button)
        }
        stack.addArrangedSubview(status)
        let content = window.contentView!
        content.addSubview(stack)
        stack.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([stack.leadingAnchor.constraint(equalTo: content.leadingAnchor, constant: 26),
                                     stack.trailingAnchor.constraint(equalTo: content.trailingAnchor, constant: -26),
                                     stack.topAnchor.constraint(equalTo: content.topAnchor, constant: 22)])
        window.center()
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if working { NSSound.beep(); return .terminateCancel }
        return .terminateNow
    }
    private var startupMessage: String {
        if Bundle.main.url(forResource: "healthctl-source.tar", withExtension: "gz") != nil {
            return "Starting your workspace. The first launch downloads dependencies and builds the app. This can take several minutes."
        }
        return "Starting the local workspace…"
    }
    @objc private func openJournal() { run(["start"], message: startupMessage) }
    @objc private func settings() { run(["start"], message: startupMessage, settings: true) }
    @objc private func stop() { run(["stop"], message: "Stopping the local workspace…") }
    @objc private func logs() {
        let panel = NSSavePanel()
        panel.nameFieldStringValue = "healthctl-logs-\(Int(Date().timeIntervalSince1970)).txt"
        guard panel.runModal() == .OK, let path = panel.url?.path else { return }
        run(["logs", path], message: "Saving the service logs…")
    }
    private func confirm(_ title: String, _ detail: String, _ action: String) -> Bool {
        let alert = NSAlert()
        alert.messageText = title
        alert.informativeText = detail
        alert.alertStyle = .critical
        alert.addButton(withTitle: action)
        alert.addButton(withTitle: "Cancel")
        return alert.runModal() == .alertFirstButtonReturn
    }
    @objc private func deleteWorkspace() {
        guard confirm("Delete this workspace permanently?", "Every encounter, diary entry and upload in the active workspace is deleted and can't be recovered. Back it up first if you may need it.", "Delete permanently") else { return }
        run(["delete"], message: "Deleting the workspace…")
    }
    @objc private func uninstall() {
        guard confirm("Remove all healthctl data from this Mac?", "Every workspace, its records and the app's Docker images are deleted and can't be recovered. Back up anything you need first. Afterwards you can move healthctl to the Trash.", "Remove everything") else { return }
        run(["uninstall"], message: "Removing healthctl's workspaces and images…")
    }
    @objc private func dockerLink() { NSWorkspace.shared.open(URL(string: "https://www.docker.com/products/docker-desktop/")!) }
    private func passphrase(creating: Bool) -> String? {
        let prompt = NSAlert()
        prompt.messageText = creating ? "Protect your backup" : "Unlock your backup"
        prompt.informativeText = creating ? "Enter at least 12 characters. Keep this passphrase for recovery." : "Enter the passphrase used to create this backup. Your existing workspace remains available."
        let first = NSSecureTextField(), second = NSSecureTextField()
        first.placeholderString = "Backup passphrase"
        second.placeholderString = "Repeat passphrase"
        let fields = NSStackView(views: creating ? [first,second] : [first])
        fields.orientation = .vertical
        fields.frame = NSRect(x: 0, y: 0, width: 340, height: creating ? 56 : 28)
        prompt.accessoryView = fields
        prompt.addButton(withTitle: "Continue")
        prompt.addButton(withTitle: "Cancel")
        prompt.window.initialFirstResponder = first
        guard prompt.runModal() == .alertFirstButtonReturn else { return nil }
        guard (12...1024).contains(first.stringValue.utf8.count), !first.stringValue.contains("\n"),
              !creating || first.stringValue == second.stringValue else {
            status.stringValue = "Check the passphrase length and spelling, then try again."
            return nil
        }
        return first.stringValue
    }
    @objc private func platformKey() {
        let dialog = NSAlert()
        dialog.messageText = "Platform API key"
        dialog.informativeText = "Start your workspace first. This optional key is saved privately on this Mac for OpenAI API billing and transcription. Leave it empty to remove it. Saving restarts the local app."
        let field = NSSecureTextField(frame: NSRect(x: 0, y: 0, width: 350, height: 25))
        dialog.accessoryView = field
        dialog.addButton(withTitle: "Save")
        dialog.addButton(withTitle: "Cancel")
        dialog.window.initialFirstResponder = field
        if dialog.runModal() == .alertFirstButtonReturn {
            run(["platform-key"], message: "Updating the local connection…", pass: field.stringValue)
        }
    }
    @objc private func backup() {
        let panel = NSSavePanel()
        panel.nameFieldStringValue = "healthctl-\(Int(Date().timeIntervalSince1970)).healthctl"
        guard panel.runModal() == .OK, let path = panel.url?.path, let pass = passphrase(creating: true) else { return }
        run(["backup",path], message: "Encrypting your backup…", pass: pass)
    }
    @objc private func restore() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = false
        guard panel.runModal() == .OK, let path = panel.url?.path, let pass = passphrase(creating: false) else { return }
        run(["restore",path], message: "Verifying and restoring a separate workspace…", pass: pass)
    }
    private func run(_ args: [String], message: String, pass: String = "", settings: Bool = false) {
        guard !working else { return }
        working = true
        status.stringValue = message
        buttons.forEach { $0.isEnabled = false }
        let executable = Bundle.main.resourceURL!.appendingPathComponent("manage.sh")
        DispatchQueue.global(qos: .userInitiated).async {
            let task = Process(), input = Pipe(), output = Pipe(), diagnostics = Pipe()
            task.executableURL = executable
            task.arguments = args
            task.standardInput = input
            task.standardOutput = output
            task.standardError = diagnostics
            var result = "The desktop operation could not start."
            var address: URL?
            do {
                try task.run()
                input.fileHandleForWriting.write(Data((pass + "\n").utf8))
                input.fileHandleForWriting.closeFile()
                // The helper's standard output is limited to a local URL. Drain
                // Docker progress as it arrives to avoid blocking on a full pipe.
                let log = diagnostics.fileHandleForReading.readDataToEndOfFile()
                let data = output.fileHandleForReading.readDataToEndOfFile()
                task.waitUntilExit()
                if task.terminationStatus == 0 {
                    switch args[0] {
                    case "backup": result = "Encrypted backup saved."
                    case "stop": result = "Workspace stopped. Your history is kept."
                    case "logs": result = "Logs saved."
                    case "delete": result = "Workspace deleted. Opening the journal creates a new, empty one."
                    case "uninstall": result = "healthctl's workspaces and images were removed. You can now move the app to the Trash."
                    default: result = "Local workspace ready."
                    }
                    let line = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
                    if let url = URL(string: line + (settings ? "?desktop=settings" : "")), url.host == "127.0.0.1", url.scheme == "http" { address = url }
                } else {
                    let lines = String(decoding: log, as: UTF8.self).split(separator: "\n")
                    result = String(lines.last?.prefix(240) ?? "Operation failed. Check Docker Desktop and try again.")
                }
            } catch {}
            let notice = result, destination = address
            DispatchQueue.main.async {
                self.working = false
                self.status.stringValue = notice
                self.buttons.forEach { $0.isEnabled = true }
                if let url = destination { NSWorkspace.shared.open(url) }
            }
        }
    }
}
let app = NSApplication.shared
let launcher = Launcher()
app.setActivationPolicy(.regular)
app.delegate = launcher
app.run()

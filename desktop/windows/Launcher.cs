using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Sockets;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;

[assembly: AssemblyTitle("healthctl")]
[assembly: AssemblyCompany("ctlplne studio")]
[assembly: AssemblyProduct("healthctl")]
[assembly: AssemblyVersion("0.2.0.0")]
[assembly: AssemblyFileVersion("0.2.0.5")]

internal static class Package
{
    internal static readonly string[] Files = { "source.tar.gz", "compose.yaml", "desktop.yaml", "release.txt", "LICENSE", "THIRD_PARTY_NOTICES.md", "use-notice.md", "preview.txt", "healthctl-windows-x64.spdx.json" };

    internal static Stream Open(string name)
    {
        var stream = Assembly.GetExecutingAssembly().GetManifestResourceStream(name);
        if (stream == null) throw new InvalidDataException("The download is incomplete. Download the launcher again.");
        return stream;
    }

    internal static void Extract(string directory)
    {
        Directory.CreateDirectory(directory);
        if (Directory.EnumerateFileSystemEntries(directory).Any())
            throw new IOException("Choose an empty directory for the bundled source and notices.");
        foreach (var name in Files)
            using (var input = Open(name))
            using (var output = new FileStream(Path.Combine(directory, name), FileMode.CreateNew, FileAccess.Write))
                input.CopyTo(output);
    }

    internal static string[] Manifest()
    {
        string[] values;
        using (var reader = new StreamReader(Open("release.txt")))
            values = reader.ReadToEnd().Trim().Replace("\r", "").Split('\n');
        if (values.Length != 3 || !Regex.IsMatch(values[0], "^[a-f0-9]{40}$") ||
            !Regex.IsMatch(values[1], "^postgres:[a-zA-Z0-9._-]+@sha256:[a-f0-9]{64}$"))
            throw new InvalidDataException("Invalid release manifest.");
        using (var sha = SHA256.Create())
        using (var source = Open("source.tar.gz"))
            if (BitConverter.ToString(sha.ComputeHash(source)).Replace("-", "").ToLowerInvariant() != values[2])
                throw new InvalidDataException("The bundled source checksum did not match.");
        return values;
    }
}

internal sealed class Workspace
{
    private readonly string root;
    private readonly Action<string> progress;
    private string docker, work;
    private readonly Dictionary<string, string> environment = new Dictionary<string, string>();

    internal Workspace(Action<string> progress)
    {
        this.progress = progress;
        root = Environment.GetEnvironmentVariable("HEALTHCTL_DESKTOP_STATE") ??
            Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "healthctl", "workspace");
    }

    private static string FindDocker()
    {
        bool windows = Environment.OSVersion.Platform == PlatformID.Win32NT;
        if (!windows) return "/usr/bin/docker"; // The hosted Linux compatibility drill uses the same executable.
        var folders = new List<string> {
            Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), @"Docker\Docker\resources\bin"),
            Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), @"Programs\DockerDesktop\resources\bin")
        };
        folders.AddRange((Environment.GetEnvironmentVariable("PATH") ?? "").Split(Path.PathSeparator));
        foreach (string entry in folders)
        {
            string folder = entry.Trim().Trim('"');
            if (!Path.IsPathRooted(folder)) continue;
            string file = Path.Combine(folder, "docker.exe");
            if (File.Exists(file)) return file;
        }
        throw new IOException("Install Docker Desktop, start its Linux engine, then try again.");
    }

    // Arguments are fixed switches and validated identifiers. Paths go through WorkingDirectory.
    private string Run(string arguments, string failure, bool probe = false, Stream input = null)
    {
        var info = new ProcessStartInfo(docker, arguments) { UseShellExecute = false, CreateNoWindow = true,
            WorkingDirectory = work, RedirectStandardOutput = true, RedirectStandardError = true, RedirectStandardInput = input != null };
        foreach (var pair in environment) info.EnvironmentVariables[pair.Key] = pair.Value;
        using (var process = Process.Start(info))
        {
            var stdout = process.StandardOutput.ReadToEndAsync();
            // Discard potentially large build output and never place settings or command output in support logs.
            var stderr = process.StandardError.BaseStream.CopyToAsync(Stream.Null);
            if (input != null)
            {
                using (var writer = process.StandardInput) input.CopyTo(writer.BaseStream);
            }
            process.WaitForExit();
            stderr.GetAwaiter().GetResult();
            if (process.ExitCode != 0)
            {
                if (probe) return null;
                throw new IOException(failure);
            }
            return stdout.GetAwaiter().GetResult().Trim();
        }
    }

    private static byte[] RandomBytes(int count)
    {
        var value = new byte[count];
        using (var rng = RandomNumberGenerator.Create()) rng.GetBytes(value);
        return value;
    }

    private string[] Settings()
    {
        string path = Path.Combine(root, "workspace.conf");
        if (!File.Exists(path))
        {
            var listener = new TcpListener(IPAddress.Loopback, 0);
            int port;
            listener.Start();
            try { port = ((IPEndPoint)listener.LocalEndpoint).Port; }
            finally { listener.Stop(); }
            string temp = path + "." + Guid.NewGuid().ToString("N");
            try
            {
                File.WriteAllLines(temp, new[] { "healthctl-win-" + Guid.NewGuid().ToString("N"), port.ToString(),
                    Convert.ToBase64String(RandomBytes(32)), BitConverter.ToString(RandomBytes(32)).Replace("-", "") });
                File.Move(temp, path);
            }
            finally { if (File.Exists(temp)) File.Delete(temp); }
        }
        string[] values = File.ReadAllLines(path);
        int savedPort;
        if (values.Length != 4 || !Regex.IsMatch(values[0], "^healthctl-win-[0-9a-f]{32}$") ||
            !int.TryParse(values[1], out savedPort) || savedPort < 1 || savedPort > 65535 ||
            !Regex.IsMatch(values[2], "^[A-Za-z0-9+/]{43}=$") || !Regex.IsMatch(values[3], "^[A-F0-9]{64}$"))
            throw new InvalidDataException("The workspace settings cannot be read. Keep the existing workspace files for recovery.");
        return values;
    }

    internal string Start()
    {
        var release = Package.Manifest();
        docker = FindDocker();
        if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("DOCKER_HOST")))
            throw new IOException("Use your local Docker Desktop context and clear the DOCKER_HOST override before starting healthctl.");
        Directory.CreateDirectory(root);
        FileStream locked;
        try { locked = new FileStream(Path.Combine(root, "launcher.lock"), FileMode.OpenOrCreate, FileAccess.Write, FileShare.None); }
        catch (IOException) { throw new IOException("Another launcher operation is running. Wait for it to finish."); }
        using (locked)
        {
            work = Path.Combine(root, "releases", release[0]);
            if (!Directory.Exists(work))
            {
                Directory.CreateDirectory(Path.GetDirectoryName(work));
                string staging = work + "-" + Guid.NewGuid().ToString("N");
                try { Package.Extract(staging); Directory.Move(staging, work); }
                finally { if (Directory.Exists(staging)) Directory.Delete(staging, true); }
            }
            progress("Checking Docker Desktop and its Linux engine…");
            var endpoint = Run("context inspect --format {{.Endpoints.docker.Host}}",
                "Open Docker Desktop and select its local context.");
            bool isLocal = Environment.OSVersion.Platform == PlatformID.Win32NT
                ? endpoint.StartsWith("npipe:////./pipe/", StringComparison.OrdinalIgnoreCase)
                : endpoint.StartsWith("unix:///", StringComparison.Ordinal);
            if (!isLocal) throw new IOException("Select the local Docker Desktop context to keep the journal on this computer.");
            if (Run("info --format {{.OSType}}", "Start Docker Desktop, wait for the engine, and try again.") != "linux")
                throw new IOException("Switch Docker Desktop to Linux containers, then try again.");
            var settings = Settings();
            string image = "healthctl-local/app:" + release[0];
            environment["HEALTHCTL_APP_IMAGE"] = image;
            environment["HEALTHCTL_DB_IMAGE"] = release[1];
            environment["CLINICAL_PORT"] = settings[1];
            environment["CLINICAL_DATA_KEY"] = settings[2];
            environment["POSTGRES_PASSWORD"] = settings[3];
            if (Run("image inspect --format {{.Id}} " + image, "", true) == null)
            {
                progress("Building healthctl from the included source. The first launch needs internet and can take several minutes…");
                using (var source = Package.Open("source.tar.gz"))
                    Run("build --tag " + image + " -", "The app build failed. Check internet access and Docker disk space, then retry.", false, source);
            }
            if (Run("image inspect --format {{.Id}} " + release[1], "", true) == null)
            {
                progress("Downloading the database image…");
                Run("pull " + release[1], "The database download failed. Check your connection and retry.");
            }
            progress("Starting your local journal…");
            Run("compose -p " + settings[0] + " -f compose.yaml -f desktop.yaml up -d --no-build --pull never --wait app",
                "The journal could not start. Check Docker Desktop, available disk space, and whether another app is using the workspace port. Your workspace settings are kept.");
            return "http://127.0.0.1:" + settings[1] + "/";
        }
    }

    // Manage finds Docker and an existing workspace, then runs one compose
    // command for it under the same lock as Start. It never creates a workspace.
    private string Manage(string arguments, string failure)
    {
        var release = Package.Manifest();
        docker = FindDocker();
        if (!File.Exists(Path.Combine(root, "workspace.conf"))) throw new IOException("There is no workspace yet. Open the journal first.");
        FileStream locked;
        try { locked = new FileStream(Path.Combine(root, "launcher.lock"), FileMode.OpenOrCreate, FileAccess.Write, FileShare.None); }
        catch (IOException) { throw new IOException("Another launcher operation is running. Wait for it to finish."); }
        using (locked)
        {
            work = Path.Combine(root, "releases", release[0]);
            if (!Directory.Exists(work)) throw new IOException("Open the journal once before managing the workspace.");
            var settings = Settings();
            environment["HEALTHCTL_APP_IMAGE"] = "healthctl-local/app:" + release[0];
            environment["HEALTHCTL_DB_IMAGE"] = release[1];
            environment["CLINICAL_PORT"] = settings[1];
            environment["CLINICAL_DATA_KEY"] = settings[2];
            environment["POSTGRES_PASSWORD"] = settings[3];
            return Run("compose -p " + settings[0] + " -f compose.yaml -f desktop.yaml " + arguments, failure);
        }
    }

    internal string Stop()
    {
        Manage("stop", "The workspace could not be stopped. Check Docker Desktop.");
        return "Workspace stopped. Your history is kept.";
    }

    internal string Logs()
    {
        // Service logs carry events, never clinical text.
        string text = Manage("logs --no-color --tail 300 app db", "The logs could not be read. Check Docker Desktop.");
        string file = Path.Combine(root, "healthctl-logs-" + DateTime.UtcNow.ToString("yyyyMMddHHmmss") + ".txt");
        File.WriteAllText(file, text);
        return "Logs saved to " + file;
    }

    internal string Delete()
    {
        Manage("down -v --remove-orphans", "The workspace could not be deleted. Check Docker Desktop.");
        File.Delete(Path.Combine(root, "workspace.conf"));
        return "Workspace deleted. Opening the journal creates a new, empty one.";
    }

    internal string Uninstall()
    {
        var release = Package.Manifest();
        if (File.Exists(Path.Combine(root, "workspace.conf"))) Delete();
        docker = FindDocker();
        Run("image rm healthctl-local/app:" + release[0], "", true);
        if (Directory.Exists(root)) Directory.Delete(root, true);
        return "healthctl's workspace and images were removed. You can now delete the launcher.";
    }
}

internal static class Launcher
{
    [STAThread]
    private static int Main(string[] args)
    {
        try
        {
            if (args.Length == 2 && args[0] == "--extract") { Package.Manifest(); Package.Extract(args[1]); return 0; }
            if (args.Length == 1 && args[0] == "--start") { Console.WriteLine(new Workspace(Console.Error.WriteLine).Start()); return 0; }
            if (args.Length != 0) { Console.Error.WriteLine("Use --start, --extract EMPTY_DIRECTORY, or double-click the launcher."); return 2; }
            Application.EnableVisualStyles();
            Application.SetCompatibleTextRenderingDefault(false);
            using (var window = new Form { Text = "healthctl · a ctlplne studio product", AutoSize = true,
                AutoSizeMode = AutoSizeMode.GrowAndShrink, StartPosition = FormStartPosition.CenterScreen, MaximizeBox = false })
            {
                var panel = new FlowLayoutPanel { FlowDirection = FlowDirection.TopDown, WrapContents = false, AutoSize = true, Padding = new Padding(20) };
                var title = new Label { Text = "healthctl", AutoSize = true, Font = new Font(SystemFonts.MessageBoxFont.FontFamily, 22) };
                string preview;
                using (var reader = new StreamReader(Package.Open("preview.txt"))) preview = reader.ReadToEnd().Trim();
                var note = new Label { AutoSize = true, MaximumSize = new Size(440, 0), Text = preview };
                var open = new Button { Text = "Open journal or try the demo", AutoSize = true, Padding = new Padding(6) };
                var stop = new Button { Text = "Stop the workspace", AutoSize = true };
                var logs = new Button { Text = "Save logs", AutoSize = true };
                var delete = new Button { Text = "Delete this workspace…", AutoSize = true };
                var uninstall = new Button { Text = "Remove all healthctl data…", AutoSize = true };
                var docs = new LinkLabel { Text = "Set up Docker Desktop for Windows", AutoSize = true };
                var status = new Label { MaximumSize = new Size(440, 0), AutoSize = true, Text = "Start Docker Desktop using Linux containers, then open the journal." };
                panel.Controls.AddRange(new Control[] { title, note, open, stop, logs, delete, uninstall, docs, status });
                var manage = new Button[] { open, stop, logs, delete, uninstall };
                Func<Func<Workspace, string>, Task> perform = async delegate(Func<Workspace, string> action) {
                    foreach (var b in manage) b.Enabled = false;
                    var report = new Progress<string>(message => status.Text = message);
                    try { status.Text = await Task.Run(() => action(new Workspace(((IProgress<string>)report).Report))); }
                    catch (Exception ex) { status.Text = ex.Message; }
                    finally { foreach (var b in manage) b.Enabled = true; }
                };
                stop.Click += async delegate { await perform(w => w.Stop()); };
                logs.Click += async delegate { await perform(w => w.Logs()); };
                delete.Click += async delegate {
                    if (MessageBox.Show("Every encounter, diary entry and upload in this workspace is deleted and can't be recovered. Back it up first if you may need it.",
                        "Delete this workspace permanently?", MessageBoxButtons.OKCancel, MessageBoxIcon.Warning) == DialogResult.OK)
                        await perform(w => w.Delete());
                };
                uninstall.Click += async delegate {
                    if (MessageBox.Show("The workspace, its records and the app image are deleted and can't be recovered.",
                        "Remove all healthctl data?", MessageBoxButtons.OKCancel, MessageBoxIcon.Warning) == DialogResult.OK)
                        await perform(w => w.Uninstall());
                };
                window.Controls.Add(panel);
                docs.LinkClicked += delegate { Process.Start(new ProcessStartInfo("https://docs.docker.com/desktop/setup/install/windows-install/") { UseShellExecute = true }); };
                window.FormClosing += delegate(object sender, FormClosingEventArgs e) { if (!open.Enabled) e.Cancel = true; };
                open.Click += async delegate {
                    open.Enabled = false;
                    var update = new Progress<string>(message => status.Text = message);
                    try
                    {
                        var url = await Task.Run(() => new Workspace(((IProgress<string>)update).Report).Start());
                        status.Text = "Your journal is ready at " + url;
                        Process.Start(new ProcessStartInfo(url) { UseShellExecute = true });
                    }
                    catch (Exception ex) { status.Text = ex.Message; }
                    finally { open.Enabled = true; }
                };
                Application.Run(window);
            }
            return 0;
        }
        catch (Exception ex) { Console.Error.WriteLine(ex.Message); return 1; }
    }
}

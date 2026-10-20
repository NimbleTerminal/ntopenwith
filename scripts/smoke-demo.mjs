// Exercise the real browser binary through the booba bridge under Node.
// This checks the example's behavior, not browser rendering.
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import { setTimeout as delay } from 'node:timers/promises';
vm.runInThisContext(await readFile('web/wasm_exec.js', 'utf8'));
const go = new Go();
go.argv = ['choose', '-pretend'];
let exitCode;
go.exit = code => { exitCode = code; };
const { instance } = await WebAssembly.instantiate(await readFile('web/demos/choose.wasm'), go.importObject);
const timeout = setTimeout(() => { console.error('demo timed out'); process.exit(1); }, 20000);
const running = go.run(instance);
// Waits for text to appear in what the program writes from now on.
async function expectOutput(text) {
  let output = '';
  for (let n = 0; n < 160 && !output.includes(text); n++) {
    await delay(25);
    output += bubbletea_read();
  }
  assert.ok(output.includes(text), `expected terminal output: ${text} in ${JSON.stringify(output.slice(-600))}`);
  return output;
}
// Lets the program take what was typed, drawing as it goes.
async function settle() {
  await delay(200);
  bubbletea_read();
}
try {
  for (let n = 0; typeof globalThis.bubbletea_read !== 'function' && n < 100; n++) await delay(20);
  assert.equal(typeof globalThis.bubbletea_read, 'function');
  bubbletea_resize(100, 30);
  await expectOutput('Documents');
  // Negotiate the same program-status support as the browser action bar.
  bubbletea_write('\x1b]7501;?\x1b\\');
  const advertised = await expectOutput(':msg=');
  const encoded = advertised.match(/:msg=([A-Za-z0-9+/=]+)/)?.[1];
  assert.ok(encoded, 'action status payload');
  const description = Buffer.from(encoded, 'base64').toString('utf8');
  for (const label of ['Open', 'Edit', 'Open with', 'Edit with']) {
    assert.ok(description.includes(label), `advertised control: ${label}`);
  }
  // Typed text is drawn a key at a time, so wait for it instead of reading it.
  bubbletea_write('Docu');
  await settle();
  bubbletea_write('\r');
  await expectOutput('report.pdf');
  bubbletea_write('report');
  await settle();
  bubbletea_write('\r');
  await expectOutput('Would open report.pdf with Preview');
  bubbletea_write('\x1b\r'); // Alt+Enter: the "Open with" chooser.
  await expectOutput('Open report.pdf with');
  // Tab: the Edit with list for the same file, then back. Only the words that
  // change are redrawn, so look for the footer's "Enter edit · Tab open with".
  bubbletea_write('\t');
  await expectOutput('edit · Tab open');
  bubbletea_write('\t');
  await expectOutput('open · Tab edit');
  // The terminal is sent only what changed, so each step below is checked by
  // text that is new on the screen.
  bubbletea_write('\x1b[Bd'); // Down to System default, and make it the default.
  await expectOutput("the system's default is used");
  await settle();
  bubbletea_write('\x1b'); // Esc closes the chooser.
  await expectOutput('Ctrl+E edit · Alt+Enter open with');
  bubbletea_resize(40, 10);
  await expectOutput('report');
  bubbletea_write('\x03');
  await running;
  assert.equal(exitCode, 0);
  console.log('Demo: startup, actions, filter, open a folder, open a file, the Open with chooser, Tab to Edit with and back, a saved default, resize, and quit passed');
} finally {
  clearTimeout(timeout);
}
process.exit(0);

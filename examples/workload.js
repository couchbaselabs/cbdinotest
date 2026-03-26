import http from 'cbdt:http'

export const options = {
    name: "example",
    description: "An example workload that just validates we can fetch a test endpoint",
}

export function setup() {
    // nothing to setup
}

export function teardown(setupData) {
    // nothing to tear down
}

export default function (setupData) {
    const resp = http.get(WORKER_URI + '/test');
    if (resp.status === 200) {
        return
    }

    let code = 'http_' + resp.status
    let details = context + ': ' + resp.body

    try {
        const parsed = JSON.parse(resp.body)
        if (parsed.code) {
            code = parsed.code
            details = parsed.details || context
        }
    } catch {
        // body wasn't JSON, use the raw text
    }


    throw new CodedError(code, details)
}

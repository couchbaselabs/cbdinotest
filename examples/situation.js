export const options = {
    name: "example",
    description: "An example situation that basically does nothing",
    parameters: {
        "test-param": {
            type: "string",
            default: 'test'
        },
    },
    workloads: {
        'example': {
            path: __dirname + '/workload.js',
            rate: 2,
            scoring: {
                'start': { min_success_rate: 1.00, min_ops_per_second: 1 },
                'middle': { min_success_rate: 1.00, min_ops_per_second: 1 },
                'end': { min_success_rate: 1.00, min_ops_per_second: 1 },
            },
        },
    },
}

export default function (options) {
    const testParam = options['test-param']

    Utils.sleep(1000)

    Metrics.markPhase('start')

    Workload.setup({
        test: testParam,
    })

    Utils.sleep(1000)

    Workload.start()

    Utils.sleep(2000)

    Metrics.markPhase('middle')

    Utils.sleep(2000)

    Metrics.markPhase('end')

    Workload.stop()

    Utils.sleep(1000)
}

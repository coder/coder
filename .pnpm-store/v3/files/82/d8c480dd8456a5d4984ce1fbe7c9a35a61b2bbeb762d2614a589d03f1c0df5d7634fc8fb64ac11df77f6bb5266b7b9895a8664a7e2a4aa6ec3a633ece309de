'use strict';

const assert = require('assert');
const common = require('../lib/common');
const httpsnippetGenerator = require('../lib/httpsnippetGenerator');
const operation = require('./operationFixture');

const sampleData = {
    header: {
        language_tabs: [
            'csharp',
            { php: 'PHP' }
        ]
    },
    operation: clone(operation),
    options: {}
};

const xCodeSamples = [
    {
        lang: 'C#',
        source: `
            PetStore.v1.Pet pet = new PetStore.v1.Pet();
            pet.setApiKey("your api key");
            pet.petType = PetStore.v1.Pet.TYPE_DOG;
            pet.name = "Rex";
            // set other fields
            PetStoreResponse response = pet.create();
            if (response.statusCode == HttpStatusCode.Created)
            {
                // Successfully created
            }
            else
            {
                // Something wrong -- check response for errors
                Console.WriteLine(response.getRawResponse());
            }
        `
    },
    {
        lang: 'PHP',
        source: `
            $form = new \\PetStore\\Entities\\Pet();
            $form->setPetType("Dog");
            $form->setName("Rex");
            // set other fields
            try {
                $pet = $client->pets()->create($form);
            } catch (UnprocessableEntityException $e) {
                var_dump($e->getErrors());
            }
        `
    },
    {
        lang: 'sample',
        source: 'sample code'
    }
];

describe('getCodeSamples tests', () => {
    let result, testData;

    describe('when x-code-samples are included', () => {
        beforeEach(() => {
            testData = clone(sampleData);
            testData.operation['x-code-samples'] = clone(xCodeSamples);

            result = common.getCodeSamples(testData);
        });

        it('should generate code samples from x-code-samples content', () => {
            const expected = `\`\`\`csharp\n${xCodeSamples[0].source}\n\`\`\`\n\n` +
                `\`\`\`php\n${xCodeSamples[1].source}\n\`\`\`\n\n` +
                `\`\`\`sample\n${xCodeSamples[2].source}\n\`\`\`\n\n`;

            assert.equal(result, expected);
        });

        it('should add unknown x-code-samples language to language_tabs', () => {
            const expectedLanguageTabs = [].concat(sampleData.header.language_tabs, { sample: 'sample' });

            console.log(expectedLanguageTabs);

            assert.deepEqual(testData.header.language_tabs, expectedLanguageTabs);
        });

        it('should return empty string if no code samples can be generated', () => {
            testData.operation['x-code-samples'] = [];

            result = common.getCodeSamples(testData);

            assert.equal(result, '');
        });
    });

    describe('when x-code-samples are not available', () => {
        let originalHttpsnippetGenerator;

        beforeEach(() => {
            testData = clone(sampleData);
            testData.templates = {
                code_csharp: () => `csharp-sample`,
                code_php: () => `php-sample`,
                code_nodejs: () => `nodejs-sample`,
                code_unknown: () => ''
            };

            originalHttpsnippetGenerator = httpsnippetGenerator.generate;
            httpsnippetGenerator.generate = (target, client, data) => `httpsnippet-${target}-${client}-sample`;
        });

        afterEach(() => {
            httpsnippetGenerator.generate = originalHttpsnippetGenerator;
        });

        it('should generate code samples using template files by default', () => {
            const expected = `\`\`\`csharp\ncsharp-sample\n\`\`\`\n\n` +
            `\`\`\`php\nphp-sample\n\`\`\`\n\n`;

            result = common.getCodeSamples(testData);

            assert.deepEqual(result, expected);
        });

        it('should use httpsnippet to generate code samples if it is active', () => {
            const testData = clone(sampleData);
            testData.options = { httpsnippet: true };
            const expected = `\`\`\`csharp\nhttpsnippet-csharp--sample\n\`\`\`\n\n` +
            `\`\`\`php\nhttpsnippet-php--sample\n\`\`\`\n\n`;

            result = common.getCodeSamples(testData);

            assert.deepEqual(result, expected);
        });

        it('should support specifying language custom target name', () => {
            testData.header.language_tabs[0] = 'javascript--nodejs';
            const expected = `\`\`\`javascript--nodejs\nnodejs-sample\n\`\`\`\n\n` +
            `\`\`\`php\nphp-sample\n\`\`\`\n\n`;

            result = common.getCodeSamples(testData);

            assert.deepEqual(result, expected);
        });

        it('should support specifying language custom client name', () => {
            const testData = clone(sampleData);
            testData.options = { httpsnippet: true };
            testData.header.language_tabs[0] = 'javascript--nodejs';
            testData.options.language_clients = [ { 'javascript--nodejs': 'request' } ];
            const expected = `\`\`\`javascript--nodejs\nhttpsnippet-nodejs-request-sample\n\`\`\`\n\n` +
            `\`\`\`php\nhttpsnippet-php--sample\n\`\`\`\n\n`;

            result = common.getCodeSamples(testData);

            assert.deepEqual(result, expected);
        });

        it('should return empty string if no code samples can be generated', () => {
            testData.header.language_tabs = ['unknown'];

            result = common.getCodeSamples(testData);

            assert.deepEqual(result, '');
        });
    });
});

function clone(obj) {
    return JSON.parse(JSON.stringify(obj));
}

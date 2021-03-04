if (currentBuild.buildCauses.toString().contains('BranchIndexingCause'))
{
  print "Build skipped due to trigger being Branch Indexing."
  return
}

pipeline
{
   agent none

   options
   {
      buildDiscarder logRotator(artifactDaysToKeepStr: '30', artifactNumToKeepStr: '10', daysToKeepStr: '30', numToKeepStr: '10')
      timestamps()
      withAWS(region:'eu-west-1', credentials:'aws-key-releases')
   }

   tools
   {
     go 'Go'
   }

   environment
   {
     GOPATH = '/home/jenkins/go'
     GO15VENDOREXPERIMENT=1
   }

   stages
   {
      stage('Build')
      {
          agent
          {
            label 'linux'
          }
          steps
          {
             sh 'go version'
             sh 'go mod vendor'
             sh "make test"
             sh 'make build-all'
             stash name: 'build artifacts', includes: 'build/**'
          }
       }

      stage('Package')
      {
         agent
         {
           docker
           {
             label 'linux'
             image 'torch/jenkins-fpm-cook:latest'
             args '-u jenkins:docker'
           }
         }

         steps
         {
            unstash 'build artifacts'
            sh 'make package-all'
            stash name: 'package artifacts', includes: 'dist/pkg/**'
         }

         post
         {
            success
            {
               archiveArtifacts 'dist/pkg/*'
            }
         }
      }

      stage('Release to Github/S3')
      {
         when
         {
             buildingTag()
         }

         agent
         {
           label 'linux'
         }

         environment
         {
             GITHUB_CREDS = credentials('github-access-token')
         }

         steps
         {
           echo "Releasing ${TAG_NAME} to Github..."
           unstash 'package artifacts'

           script
           {
             def RELEASE_DATA = sh returnStdout: true, script: "curl -s --user \"$GITHUB_CREDS\" -X POST --data \'{ \"tag_name\": \"${TAG_NAME}\", \"name\": \"${TAG_NAME}\", \"body\": \"Insert features here.\"}\' https://api.github.com/repos/Graylog2/collector-sidecar/releases"
             def props = readJSON text: RELEASE_DATA
             echo RELEASE_DATA
             if (props.id)
             {
               env.RELEASE_ID = props.id
             }
             else
             {
               error('Github Release ID is null.')
             }

             sh '''#!/bin/bash
                 set -x
                 for file in dist/pkg/*
                 do
                   FILENAME=$(basename $file)
                   curl -H "Authorization: token $GITHUB_CREDS" -H "Content-Type: application/octet-stream" --data-binary @dist/pkg/$FILENAME https://uploads.github.com/repos/Graylog2/collector-sidecar/releases/$RELEASE_ID/assets?name=$FILENAME
                 done
             '''

             s3Upload(workingDir:'dist/pkg', bucket:'graylog2-releases', path:"graylog-collector-sidecar/${TAG_NAME}", includePathPattern:'graylog*')

              sh "docker run --rm -v $WORKSPACE:$WORKSPACE -w $WORKSPACE/dist/chocolatey torch/jenkins-mono-choco:latest pack --version ${TAG_NAME}"
           }
         }
         post
         {
           success
           {
             script
             {
                archiveArtifacts 'dist/pkg/*.nupkg'
                cleanWs()
             }
           }
         }
      }

      stage('Release to Package Repository')
      {
         when
         {
             buildingTag()
         }

         agent
         {
           label 'packages'
         }

         steps
         {
           echo "Checking out fpm-recipes..."
           checkout poll: false, scm: [$class: 'GitSCM', branches: [[name: '*/master']], doGenerateSubmoduleConfigurations: false, extensions: [[$class: 'WipeWorkspace']], submoduleCfg: [], userRemoteConfigs: [[credentialsId: 'ea5e9782-80e6-4e2b-a6ef-d19a63f4799b', url: 'https://github.com/Graylog2/fpm-recipes.git']]]

           script
           {
             def version = getShortVersion()
             sh "gl2-build-pkg-sidecar ${version}"
           }
         }
         post
         {
           success
           {
             script
             {
                cleanWs()
             }
           }
         }
      }
   }
}

//packaging script wants 1.0, not 1.0.1
@NonCPS
def getShortVersion()
{
  script
  {
    if(env.TAG_NAME ==~ /^\d+\.\d+.*/)
    {
      def parsed_version = env.TAG_NAME=~ /^\d\.\d/
      return parsed_version.getAt(0)
    }
    else
    {
      error("Build Tag must be a version number")
    }
  }
}
